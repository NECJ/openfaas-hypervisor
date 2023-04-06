package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"openfaas-hypervisor/pkg"
	AtomicIpIterator "openfaas-hypervisor/pkg"
	AtomicIterator "openfaas-hypervisor/pkg"
	Network "openfaas-hypervisor/pkg"
	Stats "openfaas-hypervisor/pkg"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/google/uuid"
	FaasProvidertypes "github.com/openfaas/faas-provider/types"
	"golang.org/x/sys/unix"
)

const (
	bridgeIp           = "172.44.0.1"
	bridgeMask         = "16"
	bridgeName         = "ofhbr"
	tapBaseName        = "ofhtap"
	kernelImagePath    = "microvms/vmlinux"
	rootfsPathTemplate = "microvms/%s/rootfs.ext4"
	networkName        = "funcnet"
	ifName             = "veth0"
	kernelPathTemplate = "unikernels/%s/build/httpreply_kvm-x86_64"
	firecrackerBinPath = "./firecracker"
)

// Enum to determine which mode the hypervisor is running in
type OFHTYPE int64

const (
	UNIKERNEL = iota
	MICROVM   = iota
	CONTAINER = iota
)

var ofhtype OFHTYPE

// Maps from function instance IP to function metadata
var functionInstanceMetadata sync.Map

// Maps from function name to a pool of ready instances
var readyFunctionInstances map[string]*pkg.VmPool = make(map[string]*pkg.VmPool)
var functionReadyConditions sync.Map

var ipIterator = AtomicIpIterator.ParseIP(bridgeIp)
var tapIterator = AtomicIterator.New()

var stats = Stats.NewStats()

func main() {
	// Check for kvm access
	err := unix.Access("/dev/kvm", unix.W_OK)
	if err != nil {
		fmt.Printf("Cannot access /dev/kvm. Try enabling kvm or running as root.\n")
		log.Fatal(err)
	}

	// Check for root access
	if x, y := 0, os.Getuid(); x != y {
		log.Fatal("Root acccess denied")
	}

	// Select weather using unikernels or microvms
	if os.Getenv("OFHTYPE") == "MICROVM" {
		ofhtype = MICROVM
	} else if os.Getenv("OFHTYPE") == "CONTAINER" {
		ofhtype = CONTAINER
	} else {
		ofhtype = UNIKERNEL
	}

	if ofhtype == MICROVM || ofhtype == UNIKERNEL {
		// setup network bridge
		err = Network.AddBridge(bridgeName, bridgeIp, bridgeMask)
		if err != nil {
			log.Print(err)
			shutdown()
		}
	}

	// Shutdown server properly
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		shutdown()
	}()

	// initialise readyFunctionInstances
	var vms []fs.DirEntry
	if ofhtype == MICROVM {
		vms, err = os.ReadDir("./microvms")
		println("Function instances type: microvm")
	} else if ofhtype == CONTAINER {
		vms, err = os.ReadDir("./containers")
		println("Function instances type: container")
	} else {
		vms, err = os.ReadDir("./unikernels")
		println("Function instances type: unikernel")
	}
	if err != nil {
		log.Fatal(err)
	}
	for _, vm := range vms {
		if vm.IsDir() {
			functionName := vm.Name()
			readyFunctionInstances[functionName] = pkg.NewPool(
				func() any {
					// Create channel to indicate that vm has initialised
					readyCondition := sync.NewCond(&sync.Mutex{})
					metadata := provisionFunctionInstance(functionName)
					// Store channel so that it can be accessed by /ready
					functionReadyConditions.Store(metadata.ip, readyCondition)
					// wait for instance to be ready
					readyCondition.L.Lock()
					readyCondition.Wait()
					readyCondition.L.Unlock()
					return metadata
				},
			)
		}
	}

	http.HandleFunc("/function/", invokeFunction)
	http.HandleFunc("/ready", registerInstanceReady)
	http.HandleFunc("/system/functions", getDeployedFunctions)
	http.HandleFunc("/system/functions/", getFunctionSummary)
	http.HandleFunc("/stats", getStats)
	http.HandleFunc("/preBoot/", preBoot)

	fmt.Printf("Server up!!\n")
	err = http.ListenAndServe(":8080", nil)

	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		shutdown()
	}
}

func shutdown() {
	if ofhtype == CONTAINER {
		// shutdown containers
		functionInstanceMetadata.Range(func(key, value any) bool {
			contaienrId := value.(*InstanceMetadata).containerId
			// containers seem to kill themselves
			// out, err := exec.Command(`runsc`, `kill`, contaienrId).Output()
			// if err != nil {
			// 	fmt.Printf("Failed delete container %s: %s, %s\n", contaienrId, err.(*exec.ExitError).Stderr, out)
			// }

			err := Network.UnbridgeContainer(contaienrId)
			if err != nil {
				fmt.Printf("Failed unbridge container %s: %s\n", contaienrId, err)
			}

			return true
		})
	} else {
		// shutdown VMs
		functionInstanceMetadata.Range(func(key, value any) bool {
			value.(*InstanceMetadata).process.Signal(os.Interrupt)
			value.(*InstanceMetadata).process.Wait()
			return true
		})

		// remove tap devices
		val := tapIterator.Next()
		for i := 0; i < val; i++ {
			tapName := tapBaseName + strconv.FormatInt(int64(i), 10)
			err := Network.DeleteTap(tapName)
			if err != nil {
				log.Print(err)
			}
		}

		err := Network.DeleteBridge(bridgeName)
		if err != nil {
			log.Print(err)
		}
	}

	os.Exit(0)
}

func invokeFunction(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	functionName := strings.TrimPrefix(req.URL.Path, "/function/")

	functionInstance, err := getReadyInstance(functionName)
	if err != nil {
		log.Printf("Error getting VM instance for function '%s': %s", functionName, err)
		http.Error(w, "Error getting VM instance for function", http.StatusInternalServerError)
		return
	}

	res, err := http.Get("http://" + functionInstance.ip + ":8080/invoke")
	if err != nil {
		fmt.Printf("Error invoking function: %s\n", err)
		http.Error(w, "Error invoking function", http.StatusInternalServerError)
		shutdown()
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error reading function response: %v", err)
		http.Error(w, "Error reading function response", http.StatusInternalServerError)
		return
	}
	w.Write(body)

	if os.Getenv("DISABLE_VM_REUSE") != "TRUE" {
		readyFunctionInstances[functionInstance.functionName].Put(functionInstance)
	}

	elapsed := time.Since(start)
	stats.AddFuncExecTimeNano(elapsed.Nanoseconds())
}

// Get a ready function instance and removes it from the ready list
func getReadyInstance(functionName string) (InstanceMetadata, error) {
	var readyInstance any = nil
	for readyInstance == nil {
		instancePool := readyFunctionInstances[functionName]
		if instancePool == nil {
			return InstanceMetadata{}, fmt.Errorf("Function %s does not exist.", functionName)
		}
		readyInstance = instancePool.Get()
	}
	return readyInstance.(InstanceMetadata), nil
}

// Register that a function VM has booted and is ready to be invoked
func registerInstanceReady(w http.ResponseWriter, r *http.Request) {
	instanceIP := strings.Split((*r).RemoteAddr, ":")[0]
	metadataAny, _ := functionInstanceMetadata.Load(instanceIP)
	metadata := metadataAny.(*InstanceMetadata)
	timeElapsed := time.Now().Sub(metadata.vmStartTime)
	condition, loaded := functionReadyConditions.LoadAndDelete(instanceIP)
	if loaded {
		condition.(*sync.Cond).L.Lock()
		condition.(*sync.Cond).Signal()
		condition.(*sync.Cond).L.Unlock()
	}
	// do this last to prevent locks from slowing down function execution
	stats.AddVmInitTimeNano(timeElapsed.Nanoseconds())
}

func runMicroVM(functionName string, metadata *InstanceMetadata) {
	tapName, macAddr := configureVmNetworking(metadata)
	functionInstanceMetadata.Store(metadata.ip, metadata)

	ctx := context.Background()
	// Setup socket path
	tempdir, err := ioutil.TempDir("", "openfaas-hypervisor-")
	if err != nil {
		log.Printf("Error creating firecracker socket: %s", err)
		shutdown()
	}
	socketPath := filepath.Join(tempdir, "socket")

	cmd := firecracker.VMCommandBuilder{}.WithSocketPath(socketPath).WithBin(firecrackerBinPath).Build(ctx)

	_, ipnet, _ := net.ParseCIDR(metadata.ip + "/" + bridgeMask)
	networkInterfaces := []firecracker.NetworkInterface{{
		StaticConfiguration: &firecracker.StaticNetworkConfiguration{
			MacAddress:  macAddr,
			HostDevName: tapName,
			IPConfiguration: &firecracker.IPConfiguration{
				IPAddr:  net.IPNet{IP: net.ParseIP(metadata.ip), Mask: ipnet.Mask},
				Gateway: net.ParseIP(bridgeIp),
				IfName:  "eth0",
			},
		},
	}}

	rootfsPath := fmt.Sprintf(rootfsPathTemplate, functionName)
	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: kernelImagePath,
		Drives:          firecracker.NewDrivesBuilder(rootfsPath).Build(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(1),
			MemSizeMib: firecracker.Int64(50),
		},
		NetworkInterfaces: networkInterfaces,
	}

	m, err := firecracker.NewMachine(ctx, cfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		log.Printf("failed to create new machine: %v", err)
		shutdown()
	}

	metadata.vmStartTime = time.Now()
	if err := m.Start(ctx); err != nil {
		log.Printf("failed to initialize machine: %v", err)
		shutdown()
	}

	pid, err := m.PID()
	if err != nil {
		log.Printf("failed to obtain machines PID: %v", err)
		shutdown()
	}
	metadata.process = &os.Process{Pid: pid}
}

func runUnikernel(functionName string, metadata *InstanceMetadata) {
	tapName, macAddr := configureVmNetworking(metadata)
	functionInstanceMetadata.Store(metadata.ip, metadata)

	kernelPath := fmt.Sprintf(kernelPathTemplate, functionName)
	qemuCmd := exec.Command(`qemu-system-x86_64`, `-netdev`, `tap,id=en0,ifname=`+tapName+`,script=no,downscript=no`, `-device`, `virtio-net-pci,netdev=en0,mac=`+macAddr, `-kernel`, kernelPath, `-append`, `netdev.ipv4_addr=`+metadata.ip+` netdev.ipv4_gw_addr=`+bridgeIp+` netdev.ipv4_subnet_mask=255.255.255.0 -- `+bridgeIp, `-cpu`, `host`, `-smp`, `1`, `-enable-kvm`, `-nographic`, `-m`, `10M`)
	metadata.vmStartTime = time.Now()

	err := qemuCmd.Start()
	if err != nil {
		log.Printf("Error starting qemu: %s", err)
		shutdown()
	}
	metadata.process = qemuCmd.Process
}

func runContainer(functionName string, metadata *InstanceMetadata) {
	metadata.containerId = uuid.New().String()

	// set up networking
	ip, err := Network.BridgeContainer(metadata.containerId)
	if err != nil {
		log.Print(err)
		shutdown()
	}
	metadata.ip = ip

	// create container directory
	tempdir, err := ioutil.TempDir("", "openfaas-hypervisor-")
	if err != nil {
		log.Printf("Error creating firecracker socket: %s", err)
		shutdown()
	}
	out, err := exec.Command(`cp`, `-r`, `./containers/`+functionName+`/rootfs`, filepath.Join(tempdir, "rootfs")).Output()
	if err != nil {
		log.Printf("Error copying rootfs: %s, %s\n", err.(*exec.ExitError).Stderr, out)
		shutdown()
	}
	containerConfigTemplate, err := os.ReadFile(`./containers/` + functionName + `/config-template.json`)
	if err != nil {
		log.Printf("Error reading container config template: %s", err)
		shutdown()
	}
	re := regexp.MustCompile(`<netns>`)
	err = os.WriteFile(filepath.Join(tempdir, "config.json"), re.ReplaceAll(containerConfigTemplate, []byte(metadata.containerId)), 0644)
	if err != nil {
		log.Printf("Error writing container config file: %s", err)
		shutdown()
	}

	// run container
	runscCmd := exec.Command(`runsc`, `run`, `--bundle`, tempdir, metadata.containerId)
	metadata.vmStartTime = time.Now()
	functionInstanceMetadata.Store(metadata.ip, metadata)

	err = runscCmd.Start()
	if err != nil {
		log.Printf("Error starting runsc: %s", err)
		shutdown()
	}
	metadata.process = runscCmd.Process
}

func provisionFunctionInstance(functionName string) InstanceMetadata {
	metadata := InstanceMetadata{functionName: functionName}
	if ofhtype == MICROVM {
		runMicroVM(functionName, &metadata)
	} else if ofhtype == CONTAINER {
		runContainer(functionName, &metadata)
	} else {
		runUnikernel(functionName, &metadata)
	}

	return metadata
}

func configureVmNetworking(metadata *InstanceMetadata) (string, string) {
	tapName := tapBaseName + strconv.FormatInt(int64(tapIterator.Next()), 10)

	err := Network.AddTap(tapName, bridgeName)
	if err != nil {
		log.Print(err)
		shutdown()
	}

	metadata.ip = ipIterator.Next()
	macAddr := Network.RandomMacAddress()

	return tapName, macAddr
}

// InstanceMetadata holds information about each function instance (VM)
type InstanceMetadata struct {
	ip           string
	functionName string
	vmStartTime  time.Time
	process      *os.Process
	containerId  string
}

func getDeployedFunctions(w http.ResponseWriter, r *http.Request) {
	functions := []FaasProvidertypes.FunctionStatus{}
	for functionName := range readyFunctionInstances {
		// TODO: get true values
		functions = append(functions, FaasProvidertypes.FunctionStatus{
			Name:              functionName,
			Replicas:          1,
			Image:             "None",
			AvailableReplicas: 1,
			InvocationCount:   0,
			Labels:            &(map[string]string{}),
			Annotations:       &(map[string]string{}),
			Namespace:         "openfaas",
			Secrets:           []string{},
			CreatedAt:         time.Now(),
		})
	}

	functionBytes, err := json.Marshal(functions)
	if err != nil {
		log.Printf("Failed to marshal functions: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to marshal functions"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(functionBytes)
}

func getFunctionSummary(w http.ResponseWriter, r *http.Request) {
	functionName := strings.TrimPrefix(r.URL.Path, "/system/functions/")
	function := FaasProvidertypes.FunctionStatus{
		Name:              functionName,
		Replicas:          1,
		Image:             "None",
		AvailableReplicas: 1,
		InvocationCount:   0,
		Labels:            &(map[string]string{}),
		Annotations:       &(map[string]string{}),
		Namespace:         "openfaas",
		Secrets:           []string{},
		CreatedAt:         time.Now(),
	}

	functionBytes, err := json.Marshal(function)
	if err != nil {
		log.Printf("Failed to marshal functions: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to marshal functions"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(functionBytes)
}

func getStats(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.Marshal(stats.GetStatsSummary())
	if err != nil {
		log.Printf("Failed to stats: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to marshal stats"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func preBoot(w http.ResponseWriter, r *http.Request) {
	functionName := strings.TrimPrefix(r.URL.Path, "/preBoot/")
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read number of vms to boot: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to read number of vms to boot"))
		return
	}
	number, err := strconv.Atoi(string(bodyBytes))
	if err != nil {
		log.Printf("Failed to read number of vms to boot: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to read number of vms to boot"))
		return
	}
	for i := 0; i < number; i++ {
		metadata := provisionFunctionInstance(functionName)
		readyFunctionInstances[functionName].Put(metadata)
	}
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"openfaas-hypervisor/pkg"
	AtomicIpIterator "openfaas-hypervisor/pkg"
	AtomicIterator "openfaas-hypervisor/pkg"
	Stats "openfaas-hypervisor/pkg"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	FaasProvidertypes "github.com/openfaas/faas-provider/types"
	"golang.org/x/sys/unix"
)

const (
	bridgeIp           = "172.44.0.1"
	bridgeMask         = "24"
	bridgeName         = "ofhbr"
	tapBaseName        = "ofhtap"
	kernelPathTemplate = "unikernels/%s/build/httpreply_kvm-x86_64"
)

// Maps from function instance IP to function metadata
var functionInstanceMetadata sync.Map

// Maps from function name to a pool of ready instances
var readyFunctionInstances map[string]*pkg.VmPool = make(map[string]*pkg.VmPool)
var functionReadyChannels sync.Map

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

	// setup network bridge
	out, err := exec.Command(`ip`, `link`, `add`, bridgeName, `type`, `bridge`).Output()
	if err != nil {
		log.Fatalf("Failed to create bridge: %s, %s", err.(*exec.ExitError).Stderr, out)
	}

	out, err = exec.Command(`ip`, `a`, `a`, bridgeIp+`/`+bridgeMask, `dev`, bridgeName).Output()
	if err != nil {
		log.Fatalf("Failed to assign ip to bridge: %s, %s", err.(*exec.ExitError).Stderr, out)
	}

	out, err = exec.Command(`ip`, `link`, `set`, `dev`, bridgeName, `up`).Output()
	if err != nil {
		log.Fatalf("Failed to bring bridge up: %s, %s", err.(*exec.ExitError).Stderr, out)
	}

	// Shutdown server properly
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		shutdown()
	}()

	// initialise readyFunctionInstances
	vms, err := os.ReadDir("./unikernels")
	if err != nil {
		log.Fatal(err)
	}
	for _, vm := range vms {
		if vm.IsDir() {
			functionName := vm.Name()
			readyFunctionInstances[functionName] = pkg.NewPool(
				func() any {
					// Create channel to indicate that vm has initialised
					readyChannel := make(chan string)

					metadata := provisionFunctionInstance(functionName)

					// Store channel so that it can be accessed by /ready
					functionReadyChannels.Store(metadata.ip, readyChannel)
					// wait for instance to be ready
					<-readyChannel

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

	// shutdown instances
	functionInstanceMetadata.Range(func(key, value any) bool {
		value.(InstanceMetadata).process.Kill()
		value.(InstanceMetadata).process.Wait()
		return true
	})

	// remove tap devices
	val := tapIterator.Next()
	for i := 0; i < val; i++ {
		tapName := tapBaseName + strconv.FormatInt(int64(i), 10)
		out, err := exec.Command(`ip`, `link`, `set`, `dev`, tapName, `down`).Output()
		if err != nil {
			log.Fatalf("Failed to take down %s: %s, %s", tapName, err.(*exec.ExitError).Stderr, out)
		}
		out, err = exec.Command(`ip`, `tuntap`, `del`, `dev`, tapName, `mode`, `tap`).Output()
		if err != nil {
			log.Fatalf("Failed to delete %s: %s, %s", tapName, err.(*exec.ExitError).Stderr, out)
		}
	}

	out, err := exec.Command(`ip`, `l`, `set`, `dev`, bridgeName, `down`).Output()
	if err != nil {
		log.Fatalf("Failed to take down %s: %s, %s", bridgeName, err.(*exec.ExitError).Stderr, out)
	}

	out, err = exec.Command(`brctl`, `delbr`, bridgeName).Output()
	if err != nil {
		log.Fatalf("Failed to delete %s: %s, %s", bridgeName, err.(*exec.ExitError).Stderr, out)
	}

	os.Exit(0)
}

func invokeFunction(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	functionName := strings.TrimPrefix(req.URL.Path, "/function/")

	functionInstance := getReadyInstance(functionName)

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
		readyFunctionInstances[functionInstance.name].Put(functionInstance)
	}

	elapsed := time.Since(start)
	stats.AddFuncExecTimeNano(elapsed.Nanoseconds())
}

// Get a ready function instance and removes it from the ready list
func getReadyInstance(functionName string) InstanceMetadata {
	var readyInstance any = nil
	for readyInstance == nil {
		instancePool := readyFunctionInstances[functionName]
		if instancePool == nil {
			log.Printf("Compiled function '" + functionName + "' does not exist.")
			shutdown()
		}
		readyInstance = instancePool.Get()
	}
	return readyInstance.(InstanceMetadata)
}

// Register that a function VM has booted and is ready to be invoked
func registerInstanceReady(w http.ResponseWriter, r *http.Request) {
	instanceIP := strings.Split((*r).RemoteAddr, ":")[0]
	metadataAny, _ := functionInstanceMetadata.Load(instanceIP)
	metadata := metadataAny.(InstanceMetadata)
	timeElapsed := time.Now().Sub(metadata.vmStartTime)
	channel, loaded := functionReadyChannels.LoadAndDelete(instanceIP)
	if loaded {
		channel.(chan string) <- instanceIP
		close(channel.(chan string))
	}
	// do this last to prevent locks from slowing down function execution
	stats.AddVmInitTimeNano(timeElapsed.Nanoseconds())
}

func RandomMacAddress() string {
	macAddress := "00:"
	for i := 0; i < 5; i++ {
		macAddress += fmt.Sprintf("%02x", rand.Intn(256)) + ":"
	}
	return strings.TrimRight(macAddress, ":")
}

func provisionFunctionInstance(functionName string) InstanceMetadata {

	tapName := tapBaseName + strconv.FormatInt(int64(tapIterator.Next()), 10)

	// create tap device
	out, err := exec.Command(`ip`, `tuntap`, `add`, `dev`, tapName, `mode`, `tap`).Output()
	if err != nil {
		fmt.Printf("Error creating tap device: %s, %s\n", err.(*exec.ExitError).Stderr, out)
		shutdown()
	}

	// attach tap to bridge
	out, err = exec.Command(`ip`, `link`, `set`, `dev`, tapName, `master`, bridgeName).Output()
	if err != nil {
		fmt.Printf("Error attaching tap device to bridge: %s, %s\n", err.(*exec.ExitError).Stderr, out)
		shutdown()
	}

	// bring tap up
	out, err = exec.Command(`ip`, `link`, `set`, `dev`, tapName, `up`).Output()
	if err != nil {
		fmt.Printf("Error bringing tap up: %s, %s\n", err.(*exec.ExitError).Stderr, out)
		shutdown()
	}

	metadata := InstanceMetadata{name: functionName}
	metadata.ip = ipIterator.Next()
	macAddr := RandomMacAddress()
	kernelPath := fmt.Sprintf(kernelPathTemplate, functionName)
	qemuCmd := exec.Command(`qemu-system-x86_64`, `-netdev`, `tap,id=en0,ifname=`+tapName+`,script=no,downscript=no`, `-device`, `virtio-net-pci,netdev=en0,mac=`+macAddr, `-kernel`, kernelPath, `-append`, `netdev.ipv4_addr=`+metadata.ip+` netdev.ipv4_gw_addr=`+bridgeIp+` netdev.ipv4_subnet_mask=255.255.255.0 -- `+bridgeIp, `-cpu`, `host`, `-smp`, `1`, `-enable-kvm`, `-nographic`, `-m`, `5M`)
	metadata.vmStartTime = time.Now()

	err = qemuCmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	metadata.process = qemuCmd.Process
	functionInstanceMetadata.Store(metadata.ip, metadata)
	return metadata
}

// InstanceMetadata holds information about each function instance (VM)
type InstanceMetadata struct {
	ip          string
	name        string
	vmStartTime time.Time
	vmReadyTime time.Time
	process     *os.Process
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

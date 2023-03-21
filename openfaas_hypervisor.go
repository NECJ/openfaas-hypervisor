package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	Stats "openfaas-hypervisor/pkg"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	FaasProvidertypes "github.com/openfaas/faas-provider/types"
	"golang.org/x/sys/unix"
)

const (
	kernelImagePath    = "microvms/vmlinux"
	rootfsPathTemplate = "microvms/%s/rootfs.ext4"
	// Using default cache directory to ensure collision avoidance on IP allocations
	cniCacheDir = "/var/lib/cni"
	networkName = "funcnet"
	ifName      = "veth0"
)

// Maps from function instance IP to function metadata
var functionInstanceMetadata sync.Map

// Maps from function name to a pool of ready instances
var readyFunctionInstances map[string]*sync.Pool = make(map[string]*sync.Pool)
var functionReadyChannels sync.Map

var firecrackerBinPath string
var cniConfDir string
var cniBinPath []string

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

	// Get path to firecracker binary
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	firecrackerBinPath = filepath.Join(dir, "firecracker")

	// setup cni paths
	cniConfDir = filepath.Join(dir, "cni/config")
	cniBinPath = []string{filepath.Join(dir, "cni/bin")} // CNI binaries

	// initialise readyFunctionInstances
	vms, err := os.ReadDir("./microvms")
	if err != nil {
		log.Fatal(err)
	}
	for _, vm := range vms {
		functionName := vm.Name()
		readyFunctionInstances[functionName] = &sync.Pool{
			New: func() any {
				// Create channel to indicate that vm has initialised
				readyChannel := make(chan string)

				metadata := provisionFunctionInstance(functionName)

				// Store channel so that it can be accessed by /ready
				functionReadyChannels.Store(metadata.ip, readyChannel)
				// wait for instance to be ready
				<-readyChannel

				return metadata
			},
		}
	}

	http.HandleFunc("/function/", invokeFunction)
	http.HandleFunc("/ready", registerInstanceReady)
	http.HandleFunc("/system/functions", getDeployedFunctions)
	http.HandleFunc("/system/functions/", getFunctionSummary)
	http.HandleFunc("/stats", getStats)

	fmt.Printf("Server up!!\n")
	err = http.ListenAndServe(":8080", nil)

	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}

func invokeFunction(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	functionName := strings.TrimPrefix(req.URL.Path, "/function/")

	functionInstance := getReadyInstance(functionName)

	res, err := http.Get("http://" + functionInstance.ip + ":8080/invoke")
	if err != nil {
		fmt.Printf("Error invoking function: %s\n", err)
		http.Error(w, "Error invoking function", http.StatusInternalServerError)
		os.Exit(1)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error reading function response: %v", err)
		http.Error(w, "Error reading function response", http.StatusInternalServerError)
		return
	}
	w.Write(body)

	setInstanceReady(functionInstance)
	elapsed := time.Since(start)
	stats.AddFuncExecTimeNano(elapsed.Nanoseconds())
}

// Get a ready function instance and removes it from the ready list
func getReadyInstance(functionName string) InstanceMetadata {
	var readyInstance any = nil
	for readyInstance == nil {
		instancePool := readyFunctionInstances[functionName]
		if instancePool == nil {
			log.Fatal("Compiled function '" + functionName + "' does not exist.")
		}
		readyInstance = instancePool.Get()
	}
	return readyInstance.(InstanceMetadata)
}

func setInstanceReady(functionInstance InstanceMetadata) {
	readyFunctionInstances[functionInstance.name].Put(functionInstance)
}

// Register that a function VM has booted and is ready to be invoked
func registerInstanceReady(w http.ResponseWriter, r *http.Request) {
	instanceIP := strings.Split((*r).RemoteAddr, ":")[0]
	metadataAny, _ := functionInstanceMetadata.Load(instanceIP)
	metadata := metadataAny.(InstanceMetadata)
	timeElapsed := time.Now().Sub(metadata.vmStartTime)
	setInstanceReady(metadata)
	channel, _ := functionReadyChannels.LoadAndDelete(instanceIP)
	channel.(chan string) <- instanceIP
	close(channel.(chan string))
	// do this last to prevent locks from slowing down function execution
	stats.AddVmInitTimeNano(timeElapsed.Nanoseconds())
}

func provisionFunctionInstance(functionName string) InstanceMetadata {
	ctx := context.Background()

	// Setup socket path
	tempdir, err := ioutil.TempDir("", "openfaas-hypervisor-vm")
	if err != nil {
		log.Fatal(err)
	}
	socketPath := filepath.Join(tempdir, "socket")

	cmd := firecracker.VMCommandBuilder{}.WithSocketPath(socketPath).WithBin(firecrackerBinPath).Build(ctx)

	networkInterfaces := []firecracker.NetworkInterface{{
		CNIConfiguration: &firecracker.CNIConfiguration{
			NetworkName: networkName,
			IfName:      ifName,
			ConfDir:     cniConfDir,
			BinPath:     cniBinPath,
			VMIfName:    "eth0",
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
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	metadata := InstanceMetadata{vmStartTime: time.Now(), name: functionName}
	if err := m.Start(ctx); err != nil {
		panic(fmt.Errorf("failed to initialize machine: %v", err))
	}

	metadata.ip = m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP.String()
	functionInstanceMetadata.Store(metadata.ip, metadata)
	return metadata
}

// InstanceMetadata holds information about each function instance (VM)
type InstanceMetadata struct {
	ip          string
	name        string
	vmStartTime time.Time
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

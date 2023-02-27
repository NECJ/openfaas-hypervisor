package main

import (
	"container/list"
	"context"
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

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"golang.org/x/sys/unix"
)

const (
	kernelImagePath = "vm/kernel/vmlinux"
	rootfsPath      = "vm/rootfs/rootfs.ext4"
	// Using default cache directory to ensure collision avoidance on IP allocations
	cniCacheDir = "/var/lib/cni"
	networkName = "funcnet"
	ifName      = "veth0"
)

// Maps from function instance IP to function metadata
var functionInstanceMetadata map[string]InstanceMetadata = make(map[string]InstanceMetadata)
var readyFunctionInstances = list.New()
var readyFunctionInstancesMutex sync.Mutex
var runningFunctionInstances = list.New()
var runningFunctionInstancesMutex sync.Mutex
var functionReadyChan = make(chan string)

var firecrackerBinPath string
var cniConfDir string
var cniBinPath []string

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

	http.HandleFunc("/invoke", invokeFunction)
	http.HandleFunc("/ready", registerInstanceReady)

	fmt.Printf("Server up!!\n")
	err = http.ListenAndServe(":8080", nil)

	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("server closed\n")
	} else if err != nil {
		fmt.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}

func invokeFunction(w http.ResponseWriter, r *http.Request) {
	readyInstance, runningListItem := getReadyInstance()
	fmt.Printf("IP of ready instance: %s\n", readyInstance.ip)

	fmt.Printf("Fake Invoke!!!\n")
	time.Sleep(2 * time.Second)

	setInstanceReady(runningListItem)
}

// Get a ready function instance and set it as running
func getReadyInstance() (InstanceMetadata, *list.Element) {
	var readyInstance *list.Element = nil
	var runningListItem *list.Element = nil
	for readyInstance == nil {
		readyFunctionInstancesMutex.Lock()
		readyInstance = readyFunctionInstances.Front()
		if readyInstance == nil {
			// unlock mutex, create new instance and wait for it to be ready
			readyFunctionInstancesMutex.Unlock()
			provisionFunctionInstance()
			<-functionReadyChan
		} else {
			readyFunctionInstances.Remove(readyInstance)
			runningFunctionInstancesMutex.Lock()
			runningListItem = runningFunctionInstances.PushBack(readyInstance.Value)
			runningFunctionInstancesMutex.Unlock()
			readyFunctionInstancesMutex.Unlock()
		}
	}
	return readyInstance.Value.(InstanceMetadata), runningListItem
}

func setInstanceReady(runningListItem *list.Element) {
	// remove it from running list
	runningFunctionInstancesMutex.Lock()
	runningFunctionInstances.Remove(runningListItem)
	runningFunctionInstancesMutex.Unlock()

	// add it to ready list
	readyFunctionInstancesMutex.Lock()
	readyFunctionInstances.PushBack(runningListItem.Value)
	readyFunctionInstancesMutex.Unlock()
}

// Register that a function VM has booted and is ready to be invoked
func registerInstanceReady(w http.ResponseWriter, r *http.Request) {
	instanceIP := strings.Split((*r).RemoteAddr, ":")[0]
	if metadata, ok := functionInstanceMetadata[instanceIP]; ok {
		metadata.vmReadyTime = time.Now()
		functionInstanceMetadata[instanceIP] = metadata
	}
	timeElapsed := functionInstanceMetadata[instanceIP].vmReadyTime.Sub(functionInstanceMetadata[instanceIP].vmStartTime)
	fmt.Printf("Function ready to be invoked after %s\n", timeElapsed)
	readyFunctionInstances.PushBack(functionInstanceMetadata[instanceIP])
	functionReadyChan <- instanceIP
}

func provisionFunctionInstance() {
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

	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: kernelImagePath,
		Drives:          firecracker.NewDrivesBuilder(rootfsPath).Build(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(1),
			MemSizeMib: firecracker.Int64(2048),
		},
		NetworkInterfaces: networkInterfaces,
	}

	m, err := firecracker.NewMachine(ctx, cfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		panic(fmt.Errorf("failed to create new machine: %v", err))
	}

	metadata := InstanceMetadata{vmStartTime: time.Now()}
	if err := m.Start(ctx); err != nil {
		panic(fmt.Errorf("failed to initialize machine: %v", err))
	}

	metadata.ip = m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP.String()
	functionInstanceMetadata[metadata.ip] = metadata
}

// InstanceMetadata holds information about each function instance (VM)
type InstanceMetadata struct {
	ip          string
	vmStartTime time.Time
	vmReadyTime time.Time
}

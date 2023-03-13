package main

import (
	"container/list"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	IpIterator "openfaas-hypervisor/pkg"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	bridgeIp   = "172.44.0.1"
	bridgeMask = "24"
)

// Maps from function instance IP to function metadata
var functionInstanceMetadata map[string]InstanceMetadata = make(map[string]InstanceMetadata)
var readyFunctionInstances = list.New()
var readyFunctionInstancesMutex sync.Mutex
var functionReadyChan = make(chan string)

var ipIterator = IpIterator.ParseIP(bridgeIp)
var ipIteratorLock sync.Mutex

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
	createBridgeCmd := exec.Command(`brctl`, `addbr`, `virbr0`)
	err = createBridgeCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	addIpCmd := exec.Command(`ip`, `a`, `a`, bridgeIp+`/`+bridgeMask, `dev`, `virbr0`)
	err = addIpCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	upCmd := exec.Command(`ip`, `l`, `set`, `dev`, `virbr0`, `up`)
	err = upCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		downCmd := exec.Command(`ip`, `l`, `set`, `dev`, `virbr0`, `down`)
		err = downCmd.Run()
		if err != nil {
			log.Fatal(err)
		}

		deleteBridgeCmd := exec.Command(`brctl`, `delbr`, `virbr0`)
		err = deleteBridgeCmd.Run()
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()

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

func invokeFunction(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	functionInstance := getReadyInstance()

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
	fmt.Printf("Function invoke took: %s\n", elapsed)
}

// Get a ready function instance and removes it from the ready list
func getReadyInstance() InstanceMetadata {
	var readyInstance *list.Element = nil
	for readyInstance == nil {
		readyFunctionInstancesMutex.Lock()
		readyInstance = readyFunctionInstances.Front()
		if readyInstance == nil {
			readyFunctionInstancesMutex.Unlock()
			provisionFunctionInstance()
			// wait for it to be ready
			<-functionReadyChan
		} else {
			readyFunctionInstances.Remove(readyInstance)
			readyFunctionInstancesMutex.Unlock()
		}
	}
	return readyInstance.Value.(InstanceMetadata)
}

func setInstanceReady(functionInstance InstanceMetadata) {
	readyFunctionInstancesMutex.Lock()
	readyFunctionInstances.PushBack(functionInstance)
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
	readyFunctionInstancesMutex.Lock()
	readyFunctionInstances.PushBack(functionInstanceMetadata[instanceIP])
	readyFunctionInstancesMutex.Unlock()
	functionReadyChan <- instanceIP
}

func provisionFunctionInstance() {
	metadata := InstanceMetadata{}
	ipIteratorLock.Lock()
	ipIterator.Next()
	metadata.ip = ipIterator.String()
	ipIteratorLock.Unlock()
	targetCmd := exec.Command(`qemu-system-x86_64`, `-netdev`, `bridge,id=en0,br=virbr0`, `-device`, `virtio-net-pci,netdev=en0`, `-kernel`, `unikernel/build/httpreply_kvm-x86_64`, `-append`, `netdev.ipv4_addr=`+metadata.ip+` netdev.ipv4_gw_addr=`+bridgeIp+` netdev.ipv4_subnet_mask=255.255.255.0 -- `+bridgeIp, `-cpu`, `host`, `-enable-kvm`, `-serial`, `none`, `-parallel`, `none`, `-monitor`, `none`, `-display`, `none`, `-vga`, `none`, `-daemonize`, `-m`, `4M`)
	metadata.vmStartTime = time.Now()
	err := targetCmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	functionInstanceMetadata[metadata.ip] = metadata
}

// InstanceMetadata holds information about each function instance (VM)
type InstanceMetadata struct {
	ip          string
	vmStartTime time.Time
	vmReadyTime time.Time
}

package main

import (
	"container/list"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	AtomicIpIterator "openfaas-hypervisor/pkg"
	AtomicIterator "openfaas-hypervisor/pkg"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	bridgeIp    = "172.44.0.1"
	bridgeMask  = "24"
	bridgeName  = "ofhbr"
	tapBaseName = "ofhtap"
)

// Maps from function instance IP to function metadata
var functionInstanceMetadata map[string]InstanceMetadata = make(map[string]InstanceMetadata)
var functionInstanceMetadataMutex sync.Mutex
var readyFunctionInstances = list.New()
var readyFunctionInstancesMutex sync.Mutex
var functionReadyChan = make(chan string)

var ipIterator = AtomicIpIterator.ParseIP(bridgeIp)
var tapIterator = AtomicIterator.New()

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
	createBridgeCmd := exec.Command(`ip`, `link`, `add`, bridgeName, `type`, `bridge`)
	err = createBridgeCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	addIpCmd := exec.Command(`ip`, `a`, `a`, bridgeIp+`/`+bridgeMask, `dev`, bridgeName)
	err = addIpCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	upCmd := exec.Command(`ip`, `link`, `set`, `dev`, bridgeName, `up`)
	err = upCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	// Shutdown server properly
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		shutdown()
	}()

	http.HandleFunc("/invoke", invokeFunction)
	http.HandleFunc("/ready", registerInstanceReady)

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
	for k := range functionInstanceMetadata {
		functionInstanceMetadata[k].process.Kill()
		functionInstanceMetadata[k].process.Wait()
	}

	// remove tap devices
	val := tapIterator.Next()
	for i := 0; i < val; i++ {
		downTapCmd := exec.Command(`ip`, `link`, `set`, `dev`, tapBaseName+strconv.FormatInt(int64(i), 10), `down`)
		err := downTapCmd.Run()
		if err != nil {
			log.Fatal(err)
		}
		deleteTapCmd := exec.Command(`ip`, `tuntap`, `del`, `dev`, tapBaseName+strconv.FormatInt(int64(i), 10), `mode`, `tap`)
		err = deleteTapCmd.Run()
		if err != nil {
			log.Fatal(err)
		}
	}

	downCmd := exec.Command(`ip`, `l`, `set`, `dev`, bridgeName, `down`)
	err := downCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	deleteBridgeCmd := exec.Command(`brctl`, `delbr`, bridgeName)
	err = deleteBridgeCmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	os.Exit(0)
}

func invokeFunction(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	functionInstance := getReadyInstance()

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
	functionInstanceMetadataMutex.Lock()
	if metadata, ok := functionInstanceMetadata[instanceIP]; ok {
		metadata.vmReadyTime = time.Now()
		functionInstanceMetadata[instanceIP] = metadata
	}
	functionInstanceMetadataMutex.Unlock()
	timeElapsed := functionInstanceMetadata[instanceIP].vmReadyTime.Sub(functionInstanceMetadata[instanceIP].vmStartTime)
	fmt.Printf("Function ready to be invoked after %s\n", timeElapsed)
	readyFunctionInstancesMutex.Lock()
	readyFunctionInstances.PushBack(functionInstanceMetadata[instanceIP])
	readyFunctionInstancesMutex.Unlock()
	functionReadyChan <- instanceIP
}

func RandomMacAddress() string {
	macAddress := "00:"
	for i := 0; i < 5; i++ {
		macAddress += fmt.Sprintf("%02x", rand.Intn(256)) + ":"
	}
	return strings.TrimRight(macAddress, ":")
}

func provisionFunctionInstance() {

	tapName := tapBaseName + strconv.FormatInt(int64(tapIterator.Next()), 10)

	// create tap device
	createTapCmd := exec.Command(`ip`, `tuntap`, `add`, `dev`, tapName, `mode`, `tap`)
	err := createTapCmd.Run()
	if err != nil {
		fmt.Printf("Error creating tap device: %s\n", err)
		shutdown()
	}

	// attach tap to bridge
	attachTapCmd := exec.Command(`ip`, `link`, `set`, `dev`, tapName, `master`, bridgeName)
	err = attachTapCmd.Run()
	if err != nil {
		fmt.Printf("Error attaching tap device to bridge: %s\n", err)
		shutdown()
	}

	// bring tap up
	bringTapUpCmd := exec.Command(`ip`, `link`, `set`, `dev`, tapName, `up`)
	err = bringTapUpCmd.Run()
	if err != nil {
		fmt.Printf("Error bringing tap up: %s\n", err)
		shutdown()
	}

	metadata := InstanceMetadata{}
	metadata.ip = ipIterator.Next()
	macAddr := RandomMacAddress()
	targetCmd := exec.Command(`qemu-system-x86_64`, `-netdev`, `tap,id=en0,ifname=`+tapName+`,script=no,downscript=no`, `-device`, `virtio-net-pci,netdev=en0,mac=`+macAddr, `-kernel`, `unikernel/build/httpreply_kvm-x86_64`, `-append`, `netdev.ipv4_addr=`+metadata.ip+` netdev.ipv4_gw_addr=`+bridgeIp+` netdev.ipv4_subnet_mask=255.255.255.0 -- `+bridgeIp, `-cpu`, `host`, `-enable-kvm`, `-nographic`, `-m`, `4M`)
	metadata.vmStartTime = time.Now()
	err = targetCmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	metadata.process = targetCmd.Process
	functionInstanceMetadataMutex.Lock()
	functionInstanceMetadata[metadata.ip] = metadata
	functionInstanceMetadataMutex.Unlock()
}

// InstanceMetadata holds information about each function instance (VM)
type InstanceMetadata struct {
	ip          string
	vmStartTime time.Time
	vmReadyTime time.Time
	process     *os.Process
}

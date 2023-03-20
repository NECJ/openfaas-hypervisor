package main

import (
	"encoding/json"
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
var readyFunctionInstances map[string]*sync.Pool = make(map[string]*sync.Pool)
var functionReadyChannels sync.Map

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

	// initialise readyFunctionInstances
	vms, err := os.ReadDir("./unikernels")
	if err != nil {
		log.Fatal(err)
	}
	for _, vm := range vms {
		if vm.IsDir() {
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
	}

	http.HandleFunc("/function/", invokeFunction)
	http.HandleFunc("/ready", registerInstanceReady)
	http.HandleFunc("/system/functions", getDeployedFunctions)
	http.HandleFunc("/system/functions/", getFunctionSummary)

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

	setInstanceReady(functionInstance)
	elapsed := time.Since(start)
	fmt.Printf("Function invoke took: %s\n", elapsed)
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

func setInstanceReady(functionInstance InstanceMetadata) {
	readyFunctionInstances[functionInstance.name].Put(functionInstance)
}

// Register that a function VM has booted and is ready to be invoked
func registerInstanceReady(w http.ResponseWriter, r *http.Request) {
	instanceIP := strings.Split((*r).RemoteAddr, ":")[0]
	metadataAny, ok := functionInstanceMetadata.Load(instanceIP)
	metadata := metadataAny.(InstanceMetadata)
	if ok {
		metadata.vmReadyTime = time.Now()
		functionInstanceMetadata.Store(instanceIP, metadata)
	}
	timeElapsed := metadata.vmReadyTime.Sub(metadata.vmStartTime)
	fmt.Printf("Function ready to be invoked after %s\n", timeElapsed)
	setInstanceReady(metadata)
	channel, _ := functionReadyChannels.LoadAndDelete(instanceIP)
	channel.(chan string) <- instanceIP
	close(channel.(chan string))
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

	metadata := InstanceMetadata{name: functionName}
	metadata.ip = ipIterator.Next()
	macAddr := RandomMacAddress()
	kernelPath := fmt.Sprintf(kernelPathTemplate, functionName)
	targetCmd := exec.Command(`qemu-system-x86_64`, `-netdev`, `tap,id=en0,ifname=`+tapName+`,script=no,downscript=no`, `-device`, `virtio-net-pci,netdev=en0,mac=`+macAddr, `-kernel`, kernelPath, `-append`, `netdev.ipv4_addr=`+metadata.ip+` netdev.ipv4_gw_addr=`+bridgeIp+` netdev.ipv4_subnet_mask=255.255.255.0 -- `+bridgeIp, `-cpu`, `host`, `-enable-kvm`, `-nographic`, `-m`, `4M`)
	metadata.vmStartTime = time.Now()
	err = targetCmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	metadata.process = targetCmd.Process
	go func() {
		out, err := exec.Command(`./wss.pl`, `-C`, `-t`, `-d`, `1`, strconv.FormatInt(int64(metadata.process.Pid), 10), `0.001`).Output()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", out)
	}()

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

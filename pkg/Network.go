package pkg

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strings"
)

func AddBridge(name string, ip string, mask string) error {
	out, err := exec.Command(`ip`, `link`, `add`, name, `type`, `bridge`).Output()
	if err != nil {
		return fmt.Errorf("Failed to create bridge: %s, %s", err.(*exec.ExitError).Stderr, out)
	}

	out, err = exec.Command(`ip`, `a`, `a`, ip+`/`+mask, `dev`, name).Output()
	if err != nil {
		return fmt.Errorf("Failed to assign ip to bridge: %s, %s", err.(*exec.ExitError).Stderr, out)
	}

	out, err = exec.Command(`ip`, `link`, `set`, `dev`, name, `up`).Output()
	if err != nil {
		return fmt.Errorf("Failed to bring bridge up: %s, %s", err.(*exec.ExitError).Stderr, out)
	}
	return nil
}

func DeleteBridge(name string) error {
	out, err := exec.Command(`ip`, `l`, `set`, `dev`, name, `down`).Output()
	if err != nil {
		return fmt.Errorf("Failed to take down %s: %s, %s", name, err.(*exec.ExitError).Stderr, out)
	}

	out, err = exec.Command(`brctl`, `delbr`, name).Output()
	if err != nil {
		return fmt.Errorf("Failed to delete %s: %s, %s", name, err.(*exec.ExitError).Stderr, out)
	}
	return nil
}

func AddTap(tapName string, bridgeName string) error {
	// create tap device
	out, err := exec.Command(`ip`, `tuntap`, `add`, `dev`, tapName, `mode`, `tap`).Output()
	if err != nil {
		return fmt.Errorf("Error creating tap device: %s, %s\n", err.(*exec.ExitError).Stderr, out)
	}

	// attach tap to bridge
	out, err = exec.Command(`ip`, `link`, `set`, `dev`, tapName, `master`, bridgeName).Output()
	if err != nil {
		return fmt.Errorf("Error attaching tap device to bridge: %s, %s\n", err.(*exec.ExitError).Stderr, out)
	}

	// bring tap up
	out, err = exec.Command(`ip`, `link`, `set`, `dev`, tapName, `up`).Output()
	if err != nil {
		return fmt.Errorf("Error bringing tap up: %s, %s\n", err.(*exec.ExitError).Stderr, out)
	}
	return nil
}

func DeleteTap(name string) error {
	out, err := exec.Command(`ip`, `link`, `set`, `dev`, name, `down`).Output()
	if err != nil {
		return fmt.Errorf("Failed to take down %s: %s, %s", name, err.(*exec.ExitError).Stderr, out)
	}
	out, err = exec.Command(`ip`, `tuntap`, `del`, `dev`, name, `mode`, `tap`).Output()
	if err != nil {
		return fmt.Errorf("Failed to delete %s: %s, %s", name, err.(*exec.ExitError).Stderr, out)
	}
	return nil
}

func RandomMacAddress() string {
	macAddress := "00:"
	for i := 0; i < 5; i++ {
		macAddress += fmt.Sprintf("%02x", rand.Intn(256)) + ":"
	}
	return strings.TrimRight(macAddress, ":")
}

func BridgeContainer(containerId string) (string, error) {
	out, err := exec.Command(`ip`, `netns`, `add`, containerId).Output()
	if err != nil {
		return "", fmt.Errorf("Error creating network namespace: %s, %s\n", err.(*exec.ExitError).Stderr, out)
	}

	bridgeCmd := exec.Command(`./containers/bridge`)
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("Failed to get current working directory: %s\n", err)
	}
	bridgeCmd.Env = []string{"CNI_COMMAND=ADD", "CNI_CONTAINERID=" + containerId, "CNI_NETNS=/run/netns/" + containerId, "CNI_IFNAME=eth0", "CNI_PATH=" + wd + "/containers"}
	cniConfig, err := os.Open("./containers/cni_config.json")
	if err != nil {
		return "", fmt.Errorf("Failed to open cni config file: %s\n", err)
	}
	bridgeCmd.Stdin = cniConfig

	out, err = bridgeCmd.Output()
	if err != nil {
		return "", fmt.Errorf("Failed connect container to bridge: %s, %s", err.(*exec.ExitError).Stderr, out)
	}

	var result map[string]map[string]string
	json.Unmarshal(out, &result)
	ip := strings.Split(result["ip4"]["ip"], "/")[0]
	return ip, nil
}

func UnbridgeContainer(containerId string) error {
	unbridgeCmd := exec.Command(`./containers/bridge`)
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Failed to get current working directory: %s\n", err)
	}
	unbridgeCmd.Env = []string{"CNI_COMMAND=DEL", "CNI_CONTAINERID=" + containerId, "CNI_NETNS=/run/netns/" + containerId, "CNI_IFNAME=eth0", "CNI_PATH=" + wd + "/containers"}
	cniConfig, err := os.Open("./containers/cni_config.json")
	if err != nil {
		return fmt.Errorf("Failed to open cni config file: %s\n", err)
	}
	unbridgeCmd.Stdin = cniConfig

	out, err := unbridgeCmd.Output()
	if err != nil {
		return fmt.Errorf("Failed deconnect container from bridge: %s, %s", err.(*exec.ExitError).Stderr, out)
	}

	out, err = exec.Command(`ip`, `netns`, `del`, containerId).Output()
	if err != nil {
		return fmt.Errorf("Error deleting network namespace: %s, %s\n", err.(*exec.ExitError).Stderr, out)
	}

	return nil
}

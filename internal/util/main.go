package util

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/pardnchiu/QemuRun-pve/internal/model"
)

func CheckIP(ip string) bool {
	allowIPs := os.Getenv("ALLOW_IPS")
	if allowIPs != "0.0.0.0" && !slices.Contains(strings.Split(allowIPs, ","), ip) {
		return false
	}
	return true
}

func CheckID(vmid int, running bool) (int, map[string]model.VM, error) {
	var ary = make(map[string]model.VM)

	ary, err := GetVMMap()
	if err != nil {
		return http.StatusInternalServerError, ary, fmt.Errorf("failed to get VM list: %v", err)
	}

	if ary[strconv.Itoa(vmid)].Node == "-" {
		return http.StatusBadRequest, ary, fmt.Errorf("this IP is not allowed to be controlled")
	}

	if ary[strconv.Itoa(vmid)].Running == running {
		if running {
			return http.StatusBadRequest, ary, fmt.Errorf("VM is running")
		}
		return http.StatusBadRequest, ary, fmt.Errorf("VM is not running")
	}

	return http.StatusOK, ary, nil
}

// 要塞入 v0.1.6 取代 .go_qemu_record
func GetVMMap() (map[string]model.VM, error) {
	var list = make(map[string]model.VM)

	cmd := exec.Command(
		"pvesh", "get", "/cluster/resources",
		"--type", "vm",
		"--output-format", "json",
	)
	output, err := cmd.Output()
	if err != nil {
		return list, err
	}

	var vmMap []map[string]interface{}
	if err := json.Unmarshal(output, &vmMap); err != nil {
		return list, err
	}

	for _, e := range vmMap {
		vmid := int(e["vmid"].(float64))
		name := e["name"].(string)
		node := e["node"].(string)
		status := e["status"].(string)
		cpu := e["maxcpu"].(float64)
		disk := e["maxdisk"].(float64)
		memory := e["maxmem"].(float64)
		memoryUsed := e["mem"].(float64)
		running := status == "running"

		var os string
		if tags, exists := e["tags"]; exists && tags != nil {
			os = tags.(string)
		}

		list[strconv.Itoa(vmid)] = model.VM{
			ID:         vmid,
			Name:       name,
			OS:         os,
			Running:    running,
			Node:       node,
			CPU:        int(cpu),
			Disk:       int(disk),
			Memory:     int(memory),
			MemoryUsed: int(memoryUsed),
		}
	}

	file, err := os.Open(".go_qemu_disabled")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	disabledAry := make(map[int]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		id, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		disabledAry[id] = parts[1]
	}

	if err := scanner.Err(); err != nil {
		return list, err
	}

	for vmid, name := range disabledAry {
		list[strconv.Itoa(vmid)] = model.VM{
			ID:      vmid,
			Name:    name,
			OS:      "-",
			Running: true,
			Node:    "-",
		}
	}

	return list, nil
}

func GetNodeMap() (map[string]model.Node, error) {
	var nodeMap = make(map[string]model.Node)

	cmd := exec.Command(
		"pvesh", "get", "/cluster/resources",
		"--type", "node",
		"--output-format", "json",
	)
	output, err := cmd.Output()
	if err != nil {
		return nodeMap, err
	}

	var vmMap []map[string]interface{}
	if err := json.Unmarshal(output, &vmMap); err != nil {
		return nodeMap, err
	}

	for _, e := range vmMap {
		node := e["node"].(string)
		cpu := e["maxcpu"].(float64)
		disk := e["maxdisk"].(float64)
		memory := e["maxmem"].(float64)
		status := e["status"].(string)
		running := status == "online"

		nodeMap[node] = model.Node{
			Node:      node,
			MaxCPU:    cpu,
			MaxMemory: memory,
			Disk:      disk,
			Running:   running,
		}
	}

	return nodeMap, nil
}

func GetOSUser(vmid int) (string, error) {
	vmMap, err := GetVMMap()
	if err != nil {
		return "", err
	}
	os := vmMap[strconv.Itoa(vmid)].OS
	if os == "rockylinux" {
		os = "rocky"
	}
	if os != "debian" && os != "ubuntu" && os != "rocky" {
		return "", fmt.Errorf("OS user not found")
	}
	return os, nil
}

func IncludeVM(isRunning bool, os string, disable string) bool {
	switch disable {
	// offline
	case "1":
		return !isRunning && os != "-"
	// disable
	case "0":
		return isRunning && os != "-"
	// online
	default:
		return true
	}
}

package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/pardnchiu/QemuRun-pve/internal/model"
)

func (s *Service) assignIP() (string, int, error) {
	resultChan := make(chan model.Status, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	startEnv := os.Getenv("ASSIGN_IP_START")
	start, _ := strconv.Atoi(startEnv)
	if start == 0 || start < 100 {
		start = 100
	}

	endEnv := os.Getenv("ASSIGN_IP_END")
	end, _ := strconv.Atoi(endEnv)
	if end == 0 || end > 254 {
		end = 254
	}

	if start > end {
		start, end = end, start
	}

	for start <= end {
		wg.Add(1)
		go func(vmid int) {
			defer wg.Done()
			s.checkIPAvailable(vmid, ctx, semaphore, resultChan, cancel)
		}(start)

		if start != end {
			wg.Add(1)
			go func(vmid int) {
				defer wg.Done()
				s.checkIPAvailable(vmid, ctx, semaphore, resultChan, cancel)
			}(end)
		}

		start++
		end--
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	select {
	case result := <-resultChan:
		if result.Available {
			return result.IP, result.VMID, nil
		}
	case <-time.After(10 * time.Second):
		return "", 0, fmt.Errorf("timeout: no available IP found")
	}

	return "", 0, fmt.Errorf("cannot find available IP")
}

func (s *Service) checkIPAvailable(
	vmid int,
	ctx context.Context,
	semaphore chan struct{},
	resultChan chan model.Status,
	cancel context.CancelFunc,
) {
	// check if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return
	default:
	}

	// check if we can acquire the semaphore
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // release the semaphore

	// check again if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return
	default:
	}

	// 1. check if VMID exists in configs
	for _, vmidInConfig := range s.getVMIDsInConfigs()["all"] {
		// config exists, skip this VMID
		if vmidInConfig == strconv.Itoa(vmid) {
			return
		}
	}

	// 2. check if VMID exists by qm config
	cmd := exec.Command("qm", "config", strconv.Itoa(vmid))
	cmd.Stdout = nil // suppress output
	cmd.Stderr = nil // suppress output
	// err is nil, means VM exists
	if cmd.Run() == nil {
		return
	}

	// 3. check if IP is in use
	ip := fmt.Sprintf("192.168.0.%d", vmid)
	conn, err := net.DialTimeout("tcp", ip+":22", 2*time.Second)
	if err == nil {
		conn.Close()
		return
	}

	// IP is available
	select {
	case resultChan <- model.Status{IP: ip, VMID: vmid, Available: true}:
		cancel()
	case <-ctx.Done():
	}
}

func (s *Service) getVMIDsInConfigs() map[string][]string {
	record := make(map[string][]string)
	record["all"] = []string{}

	if _, err := os.Stat("/etc/pve/nodes"); os.IsNotExist(err) {
		return record
	}

	pattern := "/etc/pve/nodes/*/qemu-server/*.conf"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return record
	}

	re := regexp.MustCompile(`/etc/pve/nodes/([\w\-\_]+)/qemu-server/(\d+)\.conf`)
	for _, path := range matches {
		matches := re.FindStringSubmatch(path)
		if len(matches) < 3 {
			continue
		}

		record[matches[1]] = append(record[matches[1]], matches[2])
		record["all"] = append(record["all"], matches[2])
	}
	return record
}

func (s *Service) getVMIDsNode(id int) (bool, string, string) {
	record := make(map[string]string)

	if _, err := os.Stat("/etc/pve/nodes"); os.IsNotExist(err) {
		return false, "", ""
	}

	pattern := "/etc/pve/nodes/*/qemu-server/*.conf"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return false, "", ""
	}

	re := regexp.MustCompile(`/etc/pve/nodes/([\w\-\_]+)/qemu-server/(\d+)\.conf`)
	for _, path := range matches {
		matches := re.FindStringSubmatch(path)
		if len(matches) < 3 {
			continue
		}

		record[matches[2]] = matches[1]
	}

	node := record[strconv.Itoa(id)]
	mainNode := os.Getenv("MAIN_NODE")
	if mainNode == "" {
		return false, "", ""
	}

	ip := os.Getenv("NODE_" + node)
	if ip == "" {
		return false, "", ""
	}

	return node == mainNode, node, ip
}

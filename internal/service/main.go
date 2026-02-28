package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/pardnchiu/QemuRun-pve/internal/model"

	"github.com/gin-gonic/gin"
)

type Service struct {
	Gateway string
}

func NewService(gateway string) *Service {
	return &Service{
		Gateway: gateway,
	}
}

func (s *Service) getStorages() map[string]bool {
	mapStorages := make(map[string]bool)

	// list all storages
	cmd := exec.Command("pvesm", "status")
	cmd.Stderr = nil
	// get command output for filtering
	output, err := cmd.Output()
	if err != nil {
		return mapStorages
	}

	/**
	 * pvesm status output example:
	 * Name 				Type 		Status 		Total 	Used 	Available 	%
	 * local-zfs 		zfspool active 		100G 		10G 	90G 				10%
	 * local 				dir 		active 		200G 		50G 	150G 				25%
	 * nfs-storage 	nfs 		inactive 	500G 		100G 	400G 				20%
	 */
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// skip header and empty lines
		if strings.Contains(line, "Name") || strings.TrimSpace(line) == "" {
			continue
		}

		// split by whitespace
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			name := fields[0]
			storageType := fields[1]
			status := fields[2]

			// list only active storages of type dir, zfspool, lvmthin, nfs
			if status == "active" && (storageType == "dir" || storageType == "zfspool" || storageType == "lvmthin" || storageType == "nfs") {
				mapStorages[name] = true
			} else {
				mapStorages[name] = false
			}
		}
	}

	/**
	 * return value example	:
	 * {
	 *  "local-zfs": 		true,
	 *  "local": 				true,
	 *  "nfs-storage": 	false
	 * }
	 */
	return mapStorages
}

func (s *Service) initConfig(config *model.Config) error {
	if config.Name == "" {
		config.Name = fmt.Sprintf("%d", config.ID)
	} else {
		config.Name += "-" + fmt.Sprintf("%d", config.ID)
	}
	if config.CPU == 0 {
		config.CPU = 2
	}
	if config.RAM == 0 {
		config.RAM = 2048
	}
	if config.Disk == "" {
		config.Disk = "16G"
	}

	if config.User == "" {
		switch config.OS {
		case "debian":
			config.User = "debian"
		case "rockylinux":
			config.User = "rocky"
		case "ubuntu":
			config.User = "ubuntu"
		}
	}
	if config.Passwd == "" {
		config.Passwd = "passwd"
	}

	return nil
}

func (s *Service) SSE(c *gin.Context, step string, status string, message string) {
	// 檢查連線是否斷開
	select {
	case <-c.Request.Context().Done():
		// 連線已斷開，記錄但不中斷程序
		return
	default:
	}

	// 嘗試發送 SSE，如果失敗就忽略
	defer func() {
		if r := recover(); r != nil {
			// SSE 發送失敗，忽略錯誤
		}
	}()

	msg := model.SSE{
		Step:    step,
		Status:  status,
		Message: message,
	}
	data := fmt.Sprintf("data: {\"step\":\"%s\",\"status\":\"%s\",\"message\":\"%s\"}\n\n",
		msg.Step, msg.Status, msg.Message)

	if flusher, ok := c.Writer.(http.Flusher); ok {
		c.Writer.WriteString(data)
		flusher.Flush()
	}
}

func (s *Service) getCommand(isMain bool, ip string, args ...string) *exec.Cmd {
	var cmd *exec.Cmd
	if isMain {
		cmd = exec.Command("qm", args...)
	} else {
		args = append([]string{
			"-o", "ConnectTimeout=10",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "BatchMode=yes",
			fmt.Sprintf("root@%s", ip),
			"qm",
		}, args...)
		cmd = exec.Command("ssh", args...)
	}
	return cmd
}

func (s *Service) runCommandSSE(c *gin.Context, cmd *exec.Cmd, step, status string) error {
	// 取得 stdout 和 stderr 的 pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// 啟動指令
	if err := cmd.Start(); err != nil {
		return err
	}

	// 讀取並串流輸出
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			s.SSE(c, step, status, fmt.Sprintf("  %s", line))
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			// 根據內容判斷是否為實際錯誤
			if strings.Contains(line, "Error:") || strings.Contains(line, "Failed:") {
				s.SSE(c, step, status, fmt.Sprintf("  Error: %s", line))
			} else {
				s.SSE(c, step, status, fmt.Sprintf("  %s", line))
			}
		}
	}()

	// 等待指令完成
	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}

func archToLevel(arch string) int {
	levels := map[string]int{
		"x86-64-v1": 1,
		"x86-64-v2": 2,
		"x86-64-v3": 3,
		"x86-64-v4": 4,
	}
	if level, ok := levels[arch]; ok {
		return level
	}
	return 1
}

func levelToArch(level int) string {
	archs := map[int]string{
		1: "x86-64-v1",
		2: "x86-64-v2",
		3: "x86-64-v3",
		4: "x86-64-v4",
	}
	if arch, ok := archs[level]; ok {
		return arch
	}
	return "x86-64-v1"
}

func (s *Service) checkCPUArch(node string) (string, error) {
	cmd := exec.Command("pvesh", "get", fmt.Sprintf("/nodes/%s/status", node), "--output-format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get node status: %v", err)
	}

	var status struct {
		CPUInfo struct {
			Flags string `json:"flags"`
		} `json:"cpuinfo"`
	}
	if err := json.Unmarshal(output, &status); err != nil {
		return "", fmt.Errorf("failed to parse node status: %v", err)
	}

	flagSet := make(map[string]bool)
	for _, flag := range strings.Fields(status.CPUInfo.Flags) {
		flagSet[flag] = true
	}

	hasAllFlags := func(required []string) bool {
		for _, flag := range required {
			if !flagSet[flag] {
				return false
			}
		}
		return true
	}

	v2Flags := []string{"cx16", "lahf_lm", "popcnt", "sse4_1", "sse4_2", "ssse3"}
	v2AESFlags := []string{"aes", "pclmulqdq"}
	v3Flags := []string{"avx", "avx2", "bmi1", "bmi2", "f16c", "fma", "movbe", "lzcnt", "xsaveopt"}
	v4Flags := []string{"avx512f", "avx512bw", "avx512cd", "avx512dq", "avx512vl"}

	if !hasAllFlags(v2Flags) {
		return "x86-64-v1", nil
	}

	hasAES := hasAllFlags(v2AESFlags)

	if hasAllFlags(v3Flags) {
		if hasAllFlags(v4Flags) {
			return "x86-64-v4", nil
		}
		return "x86-64-v3", nil
	}

	if hasAES {
		return "x86-64-v2-AES", nil
	}

	return "x86-64-v2", nil
}

func (s *Service) GetClusterCPUType() (string, error) {
	cacheFile := ".go_qemu_cpu_type"

	if data, err := os.ReadFile(cacheFile); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	cmd := exec.Command("pvesh", "get", "/nodes", "--output-format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get nodes: %v", err)
	}

	var nodes []struct {
		Node string `json:"node"`
	}

	if err := json.Unmarshal(output, &nodes); err != nil {
		return "", fmt.Errorf("failed to parse nodes: %v", err)
	}

	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes found")
	}

	minLevel := 4
	for _, node := range nodes {
		arch, err := s.checkCPUArch(node.Node)
		if err != nil {
			return "", err
		}
		level := archToLevel(arch)
		if level < minLevel {
			minLevel = level
		}
	}

	cpuType := levelToArch(minLevel)

	if err := os.WriteFile(cacheFile, []byte(cpuType), 0644); err != nil {
		return "", fmt.Errorf("failed to write CPU type cache: %v", err)
	}

	return cpuType, nil
}

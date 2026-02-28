package service

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/pardnchiu/QemuRun-pve/internal/model"

	"github.com/gin-gonic/gin"
)

func (s *Service) Install(config *model.Config, c *gin.Context) error {
	origin := c.Request.Header.Get("Origin")
	c.Header("Access-Control-Allow-Origin", origin)
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	startTime := time.Now()
	var step string

	// Preparation steps
	// > 1. assign VMID if not specified
	s.SSE(c, step, "processing", "[*] start VM installation")
	stepStart := time.Now()
	step = "preparation > checking VMID"
	if config.ID == 0 {
		_, vmid, err := s.assignIP()
		if err != nil {
			err = fmt.Errorf("[-] failed to find free VMID: %w", err)
			s.SSE(c, step, "error", err.Error())
			return err
		}
		config.ID = vmid
		elapsed := time.Since(stepStart)
		s.SSE(c, step, "success", fmt.Sprintf("[+] auto-assigned VMID: %d (%.2fs)", config.ID, elapsed.Seconds()))
	} else {
		elapsed := time.Since(stepStart)
		s.SSE(c, step, "processing", fmt.Sprintf("[*] using specified VMID: %d (%.2fs)", config.ID, elapsed.Seconds()))
	}

	// > 2. assign IP based on gateway and VMID
	step = "preparation > assigning IP"
	stepStart = time.Now()
	ipParts := strings.Split(s.Gateway, ".")
	if len(ipParts) != 4 {
		err := fmt.Errorf("[-] invalid gateway format: %s", s.Gateway)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	ipPrefix := strings.Join(ipParts[:3], ".")
	config.IP = fmt.Sprintf("%s.%d/24", ipPrefix, config.ID)
	config.Gateway = s.Gateway
	elapsed := time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] assigned IP: %s (%.2fs)", config.IP, elapsed.Seconds()))

	// 添加 CPU / RAM 最大限制
	stepStart = time.Now()
	step = "preparation > validating CPU and RAM"
	envMaxCPU := os.Getenv("VM_MAX_CPU")
	maxCPU, err := strconv.Atoi(envMaxCPU)
	if err != nil {
		maxCPU = 0
	}
	if config.CPU <= 0 {
		s.SSE(c, step, "processing", "[*] setting CPU to minimum 1")
		config.CPU = 1
	} else if maxCPU != 0 && config.CPU > maxCPU {
		s.SSE(c, step, "processing", fmt.Sprintf("[*] setting CPU to maximum %d", maxCPU))
		config.CPU = maxCPU
	}
	envMaxRAM := os.Getenv("VM_MAX_RAM")
	maxRAM, err := strconv.Atoi(envMaxRAM)
	if err != nil {
		maxRAM = 0
	}
	if config.RAM < 512 {
		s.SSE(c, step, "processing", "[*] setting RAM to minimum 512 for ensure system stability")
		config.RAM = 512
	} else if maxRAM != 0 && config.RAM > maxRAM {
		s.SSE(c, step, "processing", fmt.Sprintf("[*] setting RAM to maximum %d", maxRAM))
		config.RAM = maxRAM
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] CPU and RAM validated (%.2fs)", elapsed.Seconds()))

	// > 3. Disk size check
	stepStart = time.Now()
	step = "preparation > validating disk size"
	diskSize, _ := strconv.Atoi(strings.TrimSuffix(config.Disk, "G"))
	envMaxDisk := os.Getenv("VM_MAX_DISK")
	maxDisk, err := strconv.Atoi(envMaxDisk)
	if err != nil {
		maxDisk = 0
	}
	if diskSize < 16 {
		s.SSE(c, step, "processing", "[*] setting disk to minimum 16G")
		config.Disk = "16G"
	} else if maxDisk != 0 && diskSize >= maxDisk {
		s.SSE(c, step, "processing", fmt.Sprintf("[*] setting disk to maximum %dG", maxDisk))
		config.Disk = fmt.Sprintf("%dG", maxDisk)
	} else {
		config.Disk = fmt.Sprintf("%dG", diskSize)
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] disk size validated (%.2fs)", elapsed.Seconds()))

	// > 4. set default config values
	stepStart = time.Now()
	step = "preparation > setting default config values"
	s.SSE(c, step, "processing", "[*] set default config values")
	if err := s.initConfig(config); err != nil {
		err = fmt.Errorf("[-] failed to set default config values: %w", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] default config values set (%.2fs)", elapsed.Seconds()))

	// > 5. check storage pool existence
	stepStart = time.Now()
	step = "preparation > checking storage pool"
	config.Storage = os.Getenv("ASSIGN_STORAGE")
	s.SSE(c, step, "processing", fmt.Sprintf("[*] check storage pool: %s", config.Storage))
	if !s.getStorages()[config.Storage] {
		err := fmt.Errorf("[-] storage pool %s does not exist or is inactive", config.Storage)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] storage pool %s is available (%.2fs)", config.Storage, elapsed.Seconds()))

	// OS preparation
	// > 1. get official OS image download link
	stepStart = time.Now()
	step = "OS preparation > getting OS image"
	s.SSE(c, step, "processing", "[*] OS preparation")
	s.SSE(c, step, "processing", "[*] get OS image download link")
	imageURL, imageFilepath, err := s.getOSImage(config.OS, config.Version)
	if err != nil {
		err = fmt.Errorf("[-] failed to get OS image URL: %w", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] OS image URL obtained (%.2fs)", elapsed.Seconds()))

	// > 2. check OS image URL availability
	stepStart = time.Now()
	step = "OS preparation > validating OS image URL"
	s.SSE(c, step, "processing", "[*] check OS image URL availability")
	if err := s.checkOSImageURL(imageURL); err != nil {
		err = fmt.Errorf("[-] OS image URL is not accessible: %w", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] OS image URL is valid (%.2fs)", elapsed.Seconds()))

	// > 3. download OS image if not exists
	stepStart = time.Now()
	if _, err := os.Stat(imageFilepath); os.IsNotExist(err) {
		step = "OS preparation > downloading OS image"
		s.SSE(c, step, "processing", "[*] start downloading OS image")
		if err := s.downloadOSImage(imageURL, imageFilepath); err != nil {
			err = fmt.Errorf("[-] failed to download OS image: %w", err)
			s.SSE(c, step, "error", err.Error())
			return err
		}
		elapsed = time.Since(stepStart)
		s.SSE(c, step, "success", fmt.Sprintf("[+] completed OS image download (%.2fs)", elapsed.Seconds()))
	} else {
		step = "OS preparation > using OS image"
		elapsed = time.Since(stepStart)
		s.SSE(c, step, "success", fmt.Sprintf("[+] using OS image (%.2fs)", elapsed.Seconds()))
	}

	// SSH preparation
	stepStart = time.Now()
	step = "SSH preparation > checking SSH key"
	s.SSE(c, step, "processing", "[*] SSH preparation")
	if stop, err := s.checkUserPubkey(); err != nil {
		err = fmt.Errorf("[-] failed to check user SSH key: %w", err)
		s.SSE(c, step, "error", err.Error())
		if stop {
			return err
		}

		step = "SSH preparation > creating SSH key"
		s.SSE(c, step, "processing", "[*] create SSH key")
		if err := s.createUserSSHKeyPair(); err != nil {
			err = fmt.Errorf("[-] failed to create SSH key: %w", err)
			s.SSE(c, step, "error", err.Error())
			return err
		}
		elapsed := time.Since(stepStart)
		s.SSE(c, step, "success", fmt.Sprintf("[+] successfully created SSH key pair (%.2fs)", elapsed.Seconds()))
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] completed SSH preparation (%.2fs)", elapsed.Seconds()))

	// VM creation
	stepStart = time.Now()
	step = "VM creation > creating VM"
	s.SSE(c, step, "processing", "[*] VM creation")
	if err := s.createVM(config); err != nil {
		err = fmt.Errorf("[-] failed to create VM: %w", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] VM created successfully (%.2fs)", elapsed.Seconds()))

	// 2. import disk image
	stepStart = time.Now()
	step = "VM creation > importing disk image"
	s.SSE(c, step, "processing", "[*] import disk image")
	if err := s.importDisk(config.ID, imageFilepath, config.Storage); err != nil {
		s.clean(config)
		err = fmt.Errorf("[-] failed to import disk image: %w", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] disk image imported successfully (%.2fs)", elapsed.Seconds()))

	// VM initialization
	stepStart = time.Now()
	step = "VM initialization > initializing configuration"
	s.SSE(c, step, "processing", "[*] initializing VM")
	if err := s.initialVM(config); err != nil {
		s.clean(config)
		err = fmt.Errorf("[-] failed to initialize VM: %w", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] VM initialized successfully (%.2fs)", elapsed.Seconds()))

	// 1. Migrate VM to specified node if needed
	if config.Node != "" {
		stepStart = time.Now()
		step = "VM initialization > migrating VM"
		s.SSE(c, step, "processing", fmt.Sprintf("[*] migrate VM to node %s", config.Node))
		args := []string{
			"migrate", strconv.Itoa(config.ID), config.Node,
			"--with-local-disks",
		}
		
		cmd := exec.Command("qm", args...)

		if err := s.runCommandSSE(c, cmd, "migrating", "processing"); err != nil {
			return err
		}
		elapsed = time.Since(stepStart)
		s.SSE(c, step, "success", fmt.Sprintf("[+] VM migrated to node %s successfully (%.2fs)", config.Node, elapsed.Seconds()))
	}

	// 2. Start VM
	stepStart = time.Now()
	isMain, _, ip := s.getVMIDsNode(config.ID)
	cmdArgs := []string{
		"start", strconv.Itoa(config.ID),
	}

	cmd := s.getCommand(isMain, ip, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("[-] failed to start VM: %v, output: %s", err, string(output))
		return err
	}

	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] VM started successfully (%.2fs)", elapsed.Seconds()))

	// 3. Wait for SSH connection
	step = "VM initialization > waiting for ready"
	osUser := config.OS
	if osUser == "rockylinux" {
		osUser = "rocky"
	}

	s.SSE(c, step, "success", "[*] VM strting")

	if err := s.CheckAlive(c, osUser, config.ID); err != nil {
		err = fmt.Errorf("[-] failed to connect to VM via SSH: %w", err)
		s.SSE(c, step, "error", err.Error())
		return nil
	}

	// 4. SSH initialization
	stepStart = time.Now()
	step = "VM initialization > SSH initialization"
	s.SSE(c, step, "processing", "[*] SSH initialization")
	if err := s.initialWithSSH(config, c); err != nil {
		s.clean(config)
		err = fmt.Errorf("[-] failed to perform SSH initialization: %w", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}
	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] SSH initialization completed (%.2fs)", elapsed.Seconds()))

	// 5. Reboot VM to apply all settings
	stepStart = time.Now()
	step = "VM initialization > rebooting VM"
	cmdArgs = []string{
		"reboot", strconv.Itoa(config.ID),
	}

	cmd = s.getCommand(isMain, ip, cmdArgs...)
	output, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("[-] failed to reboot VM: %v, output: %s", err, string(output))
		return err
	}

	elapsed = time.Since(stepStart)
	s.SSE(c, step, "processing", "[+] VM rebooting")

	if err := s.CheckAlive(c, osUser, config.ID); err != nil {
		err = fmt.Errorf("[-] failed to connect to VM via SSH: %w", err)
		s.SSE(c, step, "error", err.Error())
		return nil
	}

	step = "VM initialization > finalizing"
	s.SSE(c, step, "success", fmt.Sprintf("[+] VM is ready (%.2fs)", elapsed.Seconds()))

	totalElapsed := time.Since(startTime)
	s.SSE(c, step, "success", fmt.Sprintf("[+] VM installation completed in %.2fs", totalElapsed.Seconds()))
	s.SSE(c, step, "success", fmt.Sprintf("[*] VMID: %d", config.ID))
	s.SSE(c, step, "success", fmt.Sprintf("[*] IP: %s", strings.Split(config.IP, "/")[0]))
	s.SSE(c, step, "success", fmt.Sprintf("[*] User: %s", config.User))

	return nil
}

func (s *Service) clean(config *model.Config) {
	cmd := exec.Command("qm", "stop", strconv.Itoa(config.ID), "--skiplock", "--timeout", "1")
	cmd.Run()

	time.Sleep(5 * time.Second)

	cmd = exec.Command("qm", "destroy", strconv.Itoa(config.ID), "--purge", "--skiplock")
	cmd.Run()
}

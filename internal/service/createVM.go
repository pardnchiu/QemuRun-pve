package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pardnchiu/QemuRun-pve/internal/model"

	"github.com/gin-gonic/gin"
)

func (s *Service) createVM(config *model.Config) error {
	version, err := s.GetClusterCPUType()
	if err != nil {
		version = "kvm64"
	}

	args := []string{
		"create", strconv.Itoa(config.ID),
		"--name", config.Name,
		"--cores", strconv.Itoa(config.CPU),
		// needs to be changed if nodes have different CPU architectures, can use "x86-64-v*" for compatibility
		// you can get your CPU supported types by:
		// curl -s https://gist.githubusercontent.com/pardnchiu/561ef0581911eac7aed33c898a1a2b21/raw/ec65cc25c67703d7f8aed8d2d5859665e47dc117/cputype | bash
		"--cpu", version,
		"--scsihw", "virtio-scsi-pci",
		"--memory", strconv.Itoa(config.RAM),
		"--ostype", "l26",
		"--agent", "1",
		"--net0", "virtio,bridge=vmbr0",
		"--serial0", "socket",
	}
	envBalloonMin := os.Getenv("VM_BALLOON_MIN")
	balloonMin, err := strconv.Atoi(envBalloonMin)
	if err != nil {
		balloonMin = 0
	}

	if balloonMin != 0 && config.RAM > balloonMin {
		if config.RAM >= 65536 {
			args = append(args, "--numa", "1")
			sharingMemory := 49152
			args = append(args, "--balloon", strconv.Itoa(sharingMemory))
		} else if config.RAM >= balloonMin+1024 {
			args = append(args, "--numa", "1")
			sharingMemory := config.RAM - balloonMin
			args = append(args, "--balloon", strconv.Itoa(sharingMemory))
		}
	}

	cmd := exec.Command("qm", args...)
	return cmd.Run()
}

func (s *Service) importDisk(vmid int, filepath, storage string) error {
	cmd := exec.Command("qm", "importdisk", strconv.Itoa(vmid), filepath, storage)
	return cmd.Run()
}

func (s *Service) initialVM(config *model.Config) error {
	dirHome, _ := os.UserHomeDir()
	dirSSH := filepath.Join(dirHome, ".ssh")
	list := []string{
		"id_ed25519.pub",
		"id_rsa.pub",
		"id_ecdsa.pub",
	}

	// 1. get SSH public key
	var pubkey []byte
	var err error
	for _, e := range list {
		path := filepath.Join(dirSSH, e)
		if pubkey, err = os.ReadFile(path); err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("no SSH public key found")
	}

	// 2. create a temporary file to combine multiple public keys
	file, err := os.CreateTemp("", "combined-keys-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	if _, err := file.Write(pubkey); err != nil {
		return err
	}
	if !strings.HasSuffix(string(pubkey), "\n") {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}

	// 3. append .pubkey-admin content if exists
	if adminPubkey, err := os.ReadFile(".go_qemu_pubkey_admin"); err == nil {
		if _, err := file.Write(adminPubkey); err != nil {
			return err
		}
		if !strings.HasSuffix(string(adminPubkey), "\n") {
			if _, err := file.WriteString("\n"); err != nil {
				return err
			}
		}
	}

	// 4. append user provided public key
	if _, err := file.WriteString(config.Pubkey + "\n"); err != nil {
		return err
	}
	file.Close()

	// 5. set SSH keys to VM
	cmd := exec.Command("qm", "set", strconv.Itoa(config.ID), "--sshkeys", file.Name())
	if err := cmd.Run(); err != nil {
		return err
	}

	// 6. set password to VM
	cmd = exec.Command("qm", "set", strconv.Itoa(config.ID), "--cipassword", config.Passwd)
	if err := cmd.Run(); err != nil {
		return err
	}

	// 7. disable cloud-init package upgrade to avoid breaking the initialization
	cmd = exec.Command("qm", "set", strconv.Itoa(config.ID), "--ciupgrade", "0") // 禁用套件升級
	if err := cmd.Run(); err != nil {
		return err
	}

	// 8. set os type tag to VM
	osTag := strings.ToLower(config.OS)
	cmd = exec.Command("qm", "set", strconv.Itoa(config.ID), "--tags", osTag)
	if err := cmd.Run(); err != nil {
		return err
	}

	// 9. set disk to VM
	diskConfig := fmt.Sprintf("%s:vm-%d-disk-0", config.Storage, config.ID)
	cmd = exec.Command("qm", "set", strconv.Itoa(config.ID), "--scsi0", diskConfig)
	if err := cmd.Run(); err != nil {
		return err
	}

	// 10. set cloud-init to VM
	cloudInitConfig := fmt.Sprintf("%s:cloudinit", config.Storage)
	cmd = exec.Command("qm", "set", strconv.Itoa(config.ID), "--ide2", cloudInitConfig)
	if err := cmd.Run(); err != nil {
		return err
	}

	// 11. set boot order to VM
	cmd = exec.Command("qm", "set", strconv.Itoa(config.ID), "--boot", "c", "--bootdisk", "scsi0")
	if err := cmd.Run(); err != nil {
		return err
	}

	// 12. resize disk to user specified size
	// add retry to avoid error
	for i := 0; i < 3; i++ {
		cmd = exec.Command("qm", "resize", strconv.Itoa(config.ID), "scsi0", config.Disk)
		if err := cmd.Run(); err != nil {
			if i == 2 {
				return err
			} else {
				time.Sleep(5 * time.Second)
				continue
			}
		}
	}

	// 13. set IP and gateway to VM
	ipConfig := fmt.Sprintf("ip=%s,gw=%s", config.IP, config.Gateway)
	cmd = exec.Command("qm", "set", strconv.Itoa(config.ID), "--ipconfig0", ipConfig)
	return cmd.Run()
}

func (s *Service) waitForSSH(config *model.Config) error {
	ip := strings.Split(config.IP, "/")[0]
	maxRetries := 60

	for i := 0; i < maxRetries; i++ {
		cmd := exec.Command("ssh",
			"-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "LogLevel=ERROR",
			fmt.Sprintf("%s@%s", config.User, ip),
			"echo",
			"ready",
		)
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("[-] SSH timeout")
}

func (s *Service) getMainIP() (string, error) {
	mainNode := os.Getenv("MAIN_NODE")
	if mainNode == "" {
		return "", fmt.Errorf("MAIN_NODE not found in .env")
	}

	port := os.Getenv("PORT")
	if port == "" {
		return "", fmt.Errorf("PORT not found in .env")
	}

	nodeKey := fmt.Sprintf("NODE_%s", mainNode)

	nodeValue := os.Getenv(nodeKey)
	if nodeValue == "" {
		return "", fmt.Errorf("%s not found in .env", nodeKey)
	}

	return fmt.Sprintf("%s:%s", nodeValue, port), nil
}

func (s *Service) initialWithSSH(config *model.Config, c *gin.Context) error {
	mainIP, err := s.getMainIP()
	if err != nil {
		return err
	}

	ip := strings.Split(config.IP, "/")[0]
	host := fmt.Sprintf("%s@%s", config.User, ip)
	scriptURL := fmt.Sprintf("http://%s/sh/%s_%s.sh", mainIP, config.OS, config.Version)

	var command string
	passwdRoot := os.Getenv("VM_ROOT_PASSWORD")
	if passwdRoot != "" {
		command = fmt.Sprintf("curl -fsSL %s | sudo bash -s %s", scriptURL, passwdRoot)
	} else {
		command = fmt.Sprintf("curl -fsSL %s | sudo bash", scriptURL)
	}
	cmd := exec.Command("ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ServerAliveInterval=60",
		host,
		command,
	)

	if err := s.runCommandSSE(c, cmd, "SSH initialization", "processing"); err != nil {
		return err
	}
	return nil
}

func (s *Service) CheckAlive(c *gin.Context, os string, id int) error {
	maxRetries := 60
	ipParts := strings.Split(s.Gateway, ".")
	if len(ipParts) != 4 {
		err := fmt.Errorf("[-] invalid gateway format: %s", s.Gateway)
		s.SSE(c, "preparation", "error", err.Error())
		return err
	}
	ipPrefix := strings.Join(ipParts[:3], ".")
	ip := fmt.Sprintf("%s.%d", ipPrefix, id)

	for i := 0; i < maxRetries; i++ {
		cmd := exec.Command("ssh",
			"-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "LogLevel=ERROR",
			fmt.Sprintf("%s@%s", os, ip),
			"echo",
			"ready",
		)
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("[-] SSH timeout")
}

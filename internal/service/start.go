package service

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pardnchiu/QemuRun-pve/internal/util"
)

func (s *Service) Start(c *gin.Context, vmid int) error {
	origin := c.Request.Header.Get("Origin")
	c.Header("Access-Control-Allow-Origin", origin)
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	// 1. Start VM
	step := "starting VM"
	stepStart := time.Now()
	isMain, _, ip := s.getVMIDsNode(vmid)
	cmdArgs := []string{
		"start", strconv.Itoa(vmid),
	}

	cmd := s.getCommand(isMain, ip, cmdArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("[-] failed to start VM: %v, output: %s", err, string(output))
		return err
	}

	elapsed := time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] VM starting (%.2fs)", elapsed.Seconds()))

	// 2. Wait for SSH connection
	stepStart = time.Now()
	step = "waiting for SSH"
	osUser, err := util.GetOSUser(vmid)
	if err != nil {
		err = fmt.Errorf("[-] failed to get VM list: %v", err)
		s.SSE(c, step, "error", err.Error())
		return err
	}

	if err := s.CheckAlive(c, osUser, vmid); err != nil {
		err = fmt.Errorf("[-] failed to connect to VM via SSH: %w", err)
		s.SSE(c, step, "error", err.Error())
		return nil
	}

	elapsed = time.Since(stepStart)
	s.SSE(c, step, "success", fmt.Sprintf("[+] VM is ready (%.2fs)", elapsed.Seconds()))

	// 3. Finalizing
	step = "finalizing"
	time.Sleep(5 * time.Second)
	s.SSE(c, step, "info", "[+] VM started successfully")

	return nil
}

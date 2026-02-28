package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/pardnchiu/QemuRun-pve/internal/model"
	"github.com/pardnchiu/QemuRun-pve/internal/util"
)

func (h *Handler) Install(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.Writer.WriteString("data: {\"message\": \"this IP is not allowed to perform this action\"}\n\n")
		c.Writer.Flush()
		return
	}

	var config model.Config

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, model.Response{
			Success: false,
			Message: "please check your input:" + err.Error(),
		})
		return
	}

	h.Service.Install(&config, c)

	c.Writer.WriteString("event: close\ndata: {}\n\n")
	c.Writer.Flush()
}

func (h *Handler) Start(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.Writer.WriteString("data: {\"message\": \"this IP is not allowed to perform this action\"}\n\n")
		c.Writer.Flush()
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Writer.WriteString("data: {\"message\": \"failed to start VM\"}\n\n")
		c.Writer.Flush()
		return
	}

	_, _, err = util.CheckID(vmid, true)
	if err != nil {
		c.Writer.WriteString(fmt.Sprintf("data: {\"message\": \"%v\"}\n\n", err))
		c.Writer.Flush()
		return
	}

	err = h.Service.Start(c, vmid)
	if err != nil {
		c.Writer.WriteString(fmt.Sprintf("data: {\"message\": \"failed to start VM: %v\"}\n\n", err))
		c.Writer.Flush()
		return
	}

	c.Writer.WriteString("event: close\ndata: {}\n\n")
	c.Writer.Flush()
}

func (h *Handler) Reboot(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.Writer.WriteString("data: {\"message\": \"[-] this IP is not allowed to perform this action\"}\n\n")
		c.Writer.Flush()
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Writer.WriteString("data: {\"message\": \"[-] failed to reboot VM\"}\n\n")
		c.Writer.Flush()
		return
	}

	_, _, err = util.CheckID(vmid, false)
	if err != nil {
		c.Writer.WriteString(fmt.Sprintf("data: {\"message\": \"%v\"}\n\n", err))
		c.Writer.Flush()
		return
	}

	err = h.Service.Reboot(c, vmid)
	if err != nil {
		c.Writer.WriteString(fmt.Sprintf("data: {\"message\": \"[-] failed to reboot VM: %v\"}\n\n", err))
		c.Writer.Flush()
		return
	}

	c.Writer.WriteString("event: close\ndata: {}\n\n")
	c.Writer.Flush()
}

func (h *Handler) Node(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.Writer.WriteString("data: {\"message\": \"[-] this IP is not allowed to perform this action\"}\n\n")
		c.Writer.Flush()
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Writer.WriteString("data: {\"message\": \"[-] failed to reboot VM\"}\n\n")
		c.Writer.Flush()
		return
	}

	_, _, err = util.CheckID(vmid, true)
	if err != nil {
		c.Writer.WriteString(fmt.Sprintf("data: {\"message\": \"%v\"}\n\n", err))
		c.Writer.Flush()
		return
	}

	type reqBody struct {
		Node string `json:"node" binding:"required"`
	}

	var body reqBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid request body: %v", err)
		c.Writer.WriteString(fmt.Sprintf("data: {\"message\": \"[-] invalid request body: %v\"}\n\n", err))
		c.Writer.Flush()
		return
	}

	if err := h.Service.Node(c, vmid, body.Node); err != nil {
		err = fmt.Errorf("[-] failed to perform migrate: %w", err)
		c.Writer.WriteString(fmt.Sprintf("data: {\"message\": \"[-] failed to migrate VM: %v\"}\n\n", err))
		c.Writer.Flush()
		return
	}

	c.Writer.WriteString("event: close\ndata: {}\n\n")
	c.Writer.Flush()
}

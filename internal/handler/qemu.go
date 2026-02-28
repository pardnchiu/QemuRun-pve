package handler

import (
	"net/http"
	"strconv"

	"github.com/pardnchiu/QemuRun-pve/internal/util"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Stop(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.String(http.StatusForbidden, "this IP is not allowed to perform this action\n")
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to stop VM\n")
		return
	}

	httpStatus, _, err := util.CheckID(vmid, false)
	if err != nil {
		c.String(httpStatus, err.Error())
		return
	}

	if err := h.Service.Stop(vmid); err != nil {
		c.String(http.StatusInternalServerError, "failed to stop VM: %v\n", err)
		return
	}

	c.String(http.StatusOK, "ok")
}

func (h *Handler) Shutdown(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.String(http.StatusForbidden, "this IP is not allowed to perform this action\n")
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to shutdown VM\n")
		return
	}

	httpStatus, _, err := util.CheckID(vmid, false)
	if err != nil {
		c.String(httpStatus, err.Error())
		return
	}

	if err := h.Service.Shutdown(vmid); err != nil {
		c.String(http.StatusInternalServerError, "failed to shutdown VM: %v\n", err)
		return
	}

	c.String(http.StatusOK, "ok")
}

func (h *Handler) Destroy(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.String(http.StatusForbidden, "this IP is not allowed to perform this action\n")
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to destroy VM\n")
		return
	}

	httpStatus, _, err := util.CheckID(vmid, true)
	if err != nil {
		c.String(httpStatus, err.Error())
		return
	}

	if err := h.Service.Destroy(vmid); err != nil {
		c.String(http.StatusInternalServerError, "failed to destroy VM: %v\n", err)
		return
	}

	c.String(http.StatusOK, "ok")
}

func (h *Handler) CPU(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.String(http.StatusForbidden, "this IP is not allowed to perform this action\n")
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to destroy VM\n")
		return
	}

	httpStatus, _, err := util.CheckID(vmid, true)
	if err != nil {
		c.String(httpStatus, err.Error())
		return
	}

	type reqBody struct {
		CPU int `json:"cpu" binding:"required,min=1,max=32"`
	}

	var body reqBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if err := h.Service.CPU(vmid, body.CPU); err != nil {
		c.String(http.StatusInternalServerError, "failed to edit CPU: %v\n", err)
		return
	}

	c.String(http.StatusOK, "ok")
}

func (h *Handler) Memory(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.String(http.StatusForbidden, "this IP is not allowed to perform this action\n")
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to destroy VM\n")
		return
	}

	httpStatus, _, err := util.CheckID(vmid, true)
	if err != nil {
		c.String(httpStatus, err.Error())
		return
	}

	type reqBody struct {
		Memory int `json:"memory" binding:"required,min=512,max=32768"`
	}

	var body reqBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if err := h.Service.Memory(vmid, body.Memory); err != nil {
		c.String(http.StatusInternalServerError, "failed to edit memory: %v\n", err)
		return
	}

	c.String(http.StatusOK, "ok")
}

func (h *Handler) Disk(c *gin.Context) {
	if !util.CheckIP(c.ClientIP()) {
		c.String(http.StatusForbidden, "this IP is not allowed to perform this action\n")
		return
	}

	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to destroy VM\n")
		return
	}

	httpStatus, _, err := util.CheckID(vmid, true)
	if err != nil {
		c.String(httpStatus, err.Error())
		return
	}

	type reqBody struct {
		Disk string `json:"disk"`
	}

	var body reqBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if err := h.Service.Disk(vmid, body.Disk); err != nil {
		c.String(http.StatusInternalServerError, "failed to append disk: %v\n", err)
		return
	}

	c.String(http.StatusOK, "ok")
}

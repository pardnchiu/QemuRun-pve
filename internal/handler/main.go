package handler

import (
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/pardnchiu/QemuRun-pve/internal/model"
	"github.com/pardnchiu/QemuRun-pve/internal/service"
	"github.com/pardnchiu/QemuRun-pve/internal/util"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	Service *service.Service
}

func NewHandler(service *service.Service) *Handler {
	return &Handler{
		Service: service,
	}
}

func (h *Handler) GetStatus(c *gin.Context) {
	vmid, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid VM ID"})
		return
	}

	status, err := h.Service.GetVMStatus(vmid)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to get VM status: %v", err)
		return
	}

	c.String(http.StatusOK, status)
}

func (h *Handler) GetVMList(c *gin.Context) {
	vmMap, err := util.GetVMMap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to get VM Map\n")
		return
	}
	nodeMap, err := util.GetNodeMap()
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to get Node Map\n")
		return
	}

	disable := c.Query("disable")

	list := make([]model.VM, 0)
	for _, vm := range vmMap {
		if util.IncludeVM(vm.Running, vm.OS, disable) {
			list = append(list, vm)
		}
	}

	for i := range list {
		if util.IncludeVM(list[i].Running, list[i].OS, disable) {
			if (list[i].Node == "-") || (list[i].Node == "") {
				continue
			}
			if _, exists := nodeMap[list[i].Node]; !exists {
				nodeMap[list[i].Node] = model.Node{}
			}
			n := nodeMap[list[i].Node]
			n.CPU += float64(list[i].CPU)
			n.Memory += float64(list[i].Memory)
			n.MemoryUsed += float64(list[i].MemoryUsed)
			nodeMap[list[i].Node] = n

			list[i].Memory = list[i].Memory / 1024 / 1024 / 1024
			list[i].MemoryUsed = list[i].MemoryUsed / 1024 / 1024 / 1024
			list[i].Disk = list[i].Disk / 1024 / 1024 / 1024
		}
	}

	slices.SortFunc(list, func(a, b model.VM) int {
		return a.ID - b.ID
	})

	nodeList := make([]model.Node, 0)
	for node, n := range nodeMap {
		n.CPU = math.Round(float64(n.CPU)/n.MaxCPU*10000) / 100
		n.Memory = math.Round(float64(n.Memory)/n.MaxMemory*10000) / 100
		n.MemoryUsed = math.Round(float64(n.MemoryUsed)/n.MaxMemory*10000) / 100
		n.MaxCPU = math.Round(n.MaxCPU)
		n.MaxMemory = math.Round(n.MaxMemory/1024/1024/1024*100) / 100
		n.Disk = math.Round(n.Disk/1024/1024/1024*100) / 100
		nodeMap[node] = n
		nodeList = append(nodeList, n)
	}

	slices.SortFunc(nodeList, func(a, b model.Node) int {
		return strings.Compare(a.Node, b.Node)
	})

	c.JSON(http.StatusOK, gin.H{
		"count":   len(list),
		"list":    list,
		"cluster": nodeList,
		// ! 棄用
		"data": list,
	})
}

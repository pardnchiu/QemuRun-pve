package config

import (
	"net/http"

	"github.com/pardnchiu/QemuRun-pve/internal/handler"

	"github.com/gin-gonic/gin"
)

func NewRoutes(r *gin.Engine, h *handler.Handler) {
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		api.POST("/vm/install", h.Install)
		api.GET("/vm/:id/status", h.GetStatus)
		api.GET("/vm/list", h.GetVMList)

		group := api.Group("/vm/:id")
		{
			group.POST("/set/disk", h.Disk)
			group.POST("/set/cpu", h.CPU)
			group.POST("/set/memory", h.Memory)
			group.POST("/set/node", h.Node)
			group.POST("/start", h.Start)
			group.POST("/stop", h.Stop)
			group.POST("/shutdown", h.Shutdown)
			group.POST("/reboot", h.Reboot)
			group.POST("/destroy", h.Destroy)
		}
	}
}

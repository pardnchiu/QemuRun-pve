package main

import (
	"fmt"
	"log"
	"os"

	"github.com/pardnchiu/QemuRun-pve/internal/config"
	"github.com/pardnchiu/QemuRun-pve/internal/handler"
	"github.com/pardnchiu/QemuRun-pve/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable is required")
	}

	gateway := os.Getenv("GATEWAY")
	if gateway == "" {
		log.Fatal("GATEWAY environment variable is required")
	}

	r.Use(config.CORS())

	vmService := service.NewService(gateway)
	vmHandler := handler.NewHandler(vmService)

	r.Static("/sh", "./sh")

	config.NewRoutes(r, vmHandler)
	for _, route := range r.Routes() {
		fmt.Printf("Method: %s, Path: %s\n", route.Method, route.Path)
	}

	fmt.Println("goQemu run at localhost:" + port)
	log.Fatal(r.Run(fmt.Sprintf(":%s", port)))
}

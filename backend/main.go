package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"coal-governance-backend/config"
	"coal-governance-backend/database"
	"coal-governance-backend/routes"
	"coal-governance-backend/services"
)

func main() {
	cfg := config.Load()

	database.Connect(cfg)
	defer func() {
		if database.DB != nil {
			_ = database.DB.Close()
		}
	}()

	// Start automated SLA Escalation Cron scheduler
	services.StartSLAEscalationCron(cfg)

	router := gin.Default()

	// CORS is intentionally permissive for local hackathon demo purposes
	// (frontend served statically from a different port). Tighten
	// AllowOrigins for any real deployment.
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	// Root health check endpoint for cloud orchestrators (Render, Kubernetes, etc.)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "CoalGuard AI Governance Backend",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	routes.RegisterRoutes(router, cfg)

	// Ensure upload directory exists
	_ = os.MkdirAll(cfg.UploadDir, 0755)

	// Serve uploaded evidence photos and documents statically
	router.Static("/uploads", cfg.UploadDir)

	// Serve frontend directly if present (convenient for single-port / local runs)
	if _, err := os.Stat("../frontend/index.html"); err == nil {
		router.StaticFile("/", "../frontend/index.html")
		router.StaticFile("/index.html", "../frontend/index.html")
		router.StaticFile("/login.html", "../frontend/login.html")
		router.StaticFile("/inspections.html", "../frontend/inspections.html")
		router.StaticFile("/violations.html", "../frontend/violations.html")
		router.StaticFile("/dashboard.html", "../frontend/dashboard.html")
		router.StaticFile("/mines.html", "../frontend/mines.html")
		router.StaticFile("/analytics.html", "../frontend/analytics.html")
		router.StaticFile("/corrective-actions.html", "../frontend/corrective-actions.html")
		router.StaticFile("/compliance.html", "../frontend/compliance.html")
		router.Static("/js", "../frontend/js")
		router.Static("/css", "../frontend/css")
		router.Static("/assets", "../frontend/assets")
	} else {
		router.GET("/", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"service":   "CoalGuard AI Governance Backend",
				"status":    "online",
				"health":    "/health",
				"api_health": "/api/health",
			})
		})
	}

	log.Printf("Coal Governance backend starting on port %s\n", cfg.AppPort)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

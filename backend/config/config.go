package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all environment-driven application settings.
type Config struct {
	AppPort         string
	DBHost          string
	DBPort          string
	DBUser          string
	DBPassword      string
	DBName          string
	DBSSLMode       string
	DatabaseURL     string
	JWTSecret       string
	JWTExpiryHrs    string
	UploadDir       string
	AIServiceURL    string
	SLACronSchedule string
	GeofenceRadiusM float64
}

// Load reads .env (if present) and environment variables into a Config struct.
// Sensible defaults are provided for local development, but production
// deployments must always set these via environment variables / secrets.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	radiusStr := getEnv("GEOFENCE_RADIUS_M", "500.0")
	var radius float64 = 500.0
	if parsed, err := strconv.ParseFloat(radiusStr, 64); err == nil && parsed > 0 {
		radius = parsed
	}

	cfg := &Config{
		AppPort:         getEnv("PORT", getEnv("APP_PORT", "8080")),
		DBHost:          getEnv("DB_HOST", "127.0.0.1"),
		DBPort:          getEnv("DB_PORT", "3306"),
		DBUser:          getEnv("DB_USER", "root"),
		DBPassword:      getEnv("DB_PASSWORD", ""),
		DBName:          getEnv("DB_NAME", "coal_governance"),
		DBSSLMode:       getEnv("DB_SSL_MODE", ""),
		DatabaseURL:     getEnv("DATABASE_URL", ""),
		JWTSecret:       getEnv("JWT_SECRET", "CHANGE_ME_IN_PRODUCTION"),
		JWTExpiryHrs:    getEnv("JWT_EXPIRY_HOURS", "12"),
		UploadDir:       getEnv("UPLOAD_DIR", "./uploads"),
		AIServiceURL:    getEnv("AI_SERVICE_URL", "http://localhost:5000"),
		SLACronSchedule: getEnv("SLA_CRON_SCHEDULE", "@hourly"),
		GeofenceRadiusM: radius,
	}

	if cfg.JWTSecret == "CHANGE_ME_IN_PRODUCTION" {
		log.Println("WARNING: Using default JWT secret. Set JWT_SECRET in .env for production use.")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

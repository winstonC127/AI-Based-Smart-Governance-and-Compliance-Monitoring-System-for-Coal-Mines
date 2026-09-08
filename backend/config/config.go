package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all environment-driven application settings.
type Config struct {
	AppPort      string
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	JWTSecret    string
	JWTExpiryHrs string
	UploadDir    string
	AIServiceURL string
}

// Load reads .env (if present) and environment variables into a Config struct.
// Sensible defaults are provided for local development, but production
// deployments must always set these via environment variables / secrets.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	cfg := &Config{
		AppPort:      getEnv("APP_PORT", "8080"),
		DBHost:       getEnv("DB_HOST", "127.0.0.1"),
		DBPort:       getEnv("DB_PORT", "3306"),
		DBUser:       getEnv("DB_USER", "root"),
		DBPassword:   getEnv("DB_PASSWORD", ""),
		DBName:       getEnv("DB_NAME", "coal_governance"),
		JWTSecret:    getEnv("JWT_SECRET", "CHANGE_ME_IN_PRODUCTION"),
		JWTExpiryHrs: getEnv("JWT_EXPIRY_HOURS", "12"),
		UploadDir:    getEnv("UPLOAD_DIR", "./uploads"),
		AIServiceURL: getEnv("AI_SERVICE_URL", "http://localhost:5000"),
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

package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"coal-governance-backend/config"
)

// DB is the shared, pooled MySQL connection used across the application.
var DB *sql.DB

// Connect opens a connection pool to MySQL and verifies it with a ping.
func Connect(cfg *config.Config) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=Local",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(10)
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Connected to MySQL database:", cfg.DBName)
	RunMigrations()
}

// RunMigrations checks and adds required workflow and OCR columns to the documents table.
func RunMigrations() {
	columns := []string{
		"ALTER TABLE documents ADD COLUMN workflow_status VARCHAR(50) DEFAULT 'PENDING_REVIEW'",
		"ALTER TABLE documents ADD COLUMN mine_code VARCHAR(50) NULL",
		"ALTER TABLE documents ADD COLUMN inspector_name VARCHAR(100) NULL",
		"ALTER TABLE documents ADD COLUMN inspection_date DATE NULL",
		"ALTER TABLE documents ADD COLUMN compliance_status VARCHAR(50) DEFAULT 'COMPLIANT'",
		"ALTER TABLE documents ADD COLUMN violation_details TEXT NULL",
		"ALTER TABLE documents ADD COLUMN risk_level VARCHAR(30) DEFAULT 'LOW'",
		"ALTER TABLE documents ADD COLUMN corrective_action TEXT NULL",
		"ALTER TABLE documents ADD COLUMN due_date DATE NULL",
		"ALTER TABLE documents ADD COLUMN regulatory_reference VARCHAR(255) NULL",
		"ALTER TABLE documents ADD COLUMN ocr_data_json JSON NULL",
		"ALTER TABLE documents ADD COLUMN reviewed_by INT NULL",
		"ALTER TABLE documents ADD COLUMN reviewed_at DATETIME NULL",
		"ALTER TABLE documents ADD COLUMN approved_by INT NULL",
		"ALTER TABLE documents ADD COLUMN approved_at DATETIME NULL",
		"ALTER TABLE documents ADD COLUMN verified_by INT NULL",
		"ALTER TABLE documents ADD COLUMN verified_at DATETIME NULL",
	}

	for _, stmt := range columns {
		_, _ = DB.Exec(stmt) // Ignore error if column already exists
	}
	log.Println("Database schema migrations verified.")
}

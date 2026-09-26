package database

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"

	"coal-governance-backend/config"
)

// DB is the shared, pooled MySQL connection used across the application.
var DB *sql.DB

// parseDatabaseURL converts mysql://user:pass@host:port/dbname to go-sql-driver DSN
func parseDatabaseURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	user := u.User.Username()
	password, _ := u.User.Password()
	host := u.Host
	dbName := strings.TrimPrefix(u.Path, "/")
	
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4&loc=Local&tls=custom",
		user, password, host, dbName)
}

// Connect opens a connection pool to MySQL and verifies it with a ping.
func Connect(cfg *config.Config) {
	// Register custom TLS config with InsecureSkipVerify for cloud-hosted MySQL (Aiven, Render, etc.)
	_ = mysql.RegisterTLSConfig("custom", &tls.Config{
		InsecureSkipVerify: true,
	})

	var dsn string
	if cfg.DatabaseURL != "" {
		dsn = parseDatabaseURL(cfg.DatabaseURL)
	} else {
		sslParam := ""
		if cfg.DBSSLMode != "" {
			if cfg.DBSSLMode == "require" || cfg.DBSSLMode == "true" || cfg.DBSSLMode == "skip-verify" {
				sslParam = "&tls=custom"
			} else {
				sslParam = "&tls=" + cfg.DBSSLMode
			}
		} else if cfg.DBHost != "127.0.0.1" && cfg.DBHost != "localhost" && cfg.DBHost != "mysql" {
			// Remote cloud database: default to custom TLS with skip-verify
			sslParam = "&tls=custom"
		}

		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=Local%s",
			cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, sslParam)
	}

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
		"ALTER TABLE violations ADD COLUMN escalation_level INT DEFAULT 1",
		"ALTER TABLE violations ADD COLUMN sla_hours INT DEFAULT 48",
		"ALTER TABLE violations ADD COLUMN escalated_at TIMESTAMP NULL",
		"ALTER TABLE grievances ADD COLUMN escalation_level INT DEFAULT 1",
		"ALTER TABLE grievances ADD COLUMN sla_hours INT DEFAULT 48",
		"ALTER TABLE grievances ADD COLUMN escalated_at TIMESTAMP NULL",
		"ALTER TABLE attendance ADD COLUMN checkin_lat DECIMAL(10,6) NULL",
		"ALTER TABLE attendance ADD COLUMN checkin_lng DECIMAL(10,6) NULL",
		"ALTER TABLE attendance ADD COLUMN distance_from_mine_m DECIMAL(8,2) NULL",
		"ALTER TABLE attendance ADD COLUMN is_mock_location BOOLEAN DEFAULT FALSE",
		"ALTER TABLE attendance ADD COLUMN device_uptime_ms BIGINT NULL",
		"ALTER TABLE attendance ADD COLUMN client_reported_time TIMESTAMP NULL",
		"ALTER TABLE attendance ADD COLUMN tamper_flag BOOLEAN DEFAULT FALSE",
		"ALTER TABLE anomalies ADD COLUMN worker_id INT NULL",
		"ALTER TABLE inspections ADD COLUMN void_reason VARCHAR(255) NULL",
		"ALTER TABLE inspections ADD COLUMN voided_by INT NULL",
		"ALTER TABLE inspections ADD COLUMN voided_at TIMESTAMP NULL",
		"ALTER TABLE inspections MODIFY COLUMN status ENUM('DRAFT','SUBMITTED','REVIEWED','APPROVED','VOIDED') DEFAULT 'DRAFT'",
		"ALTER TABLE observations MODIFY COLUMN observation TEXT NULL",
		"ALTER TABLE corrective_actions ADD COLUMN evidence_photo_path VARCHAR(255) NULL",
		"ALTER TABLE corrective_actions ADD COLUMN resolution_gps_latitude DECIMAL(10,6) NULL",
		"ALTER TABLE corrective_actions ADD COLUMN resolution_gps_longitude DECIMAL(10,6) NULL",
		"ALTER TABLE corrective_actions ADD COLUMN resolution_notes TEXT NULL",
	}

	for _, stmt := range columns {
		_, _ = DB.Exec(stmt) // Ignore error if column already exists
	}

	tables := []string{
		`CREATE TABLE IF NOT EXISTS attendance_checkin_events (
			id                  BIGINT PRIMARY KEY AUTO_INCREMENT,
			mine_id             INT NOT NULL,
			worker_id           INT NOT NULL,
			lat                 DECIMAL(10,6) NOT NULL,
			lng                 DECIMAL(10,6) NOT NULL,
			distance_from_mine_m DECIMAL(8,2) NOT NULL,
			event_type          ENUM('CHECKIN','CHECKOUT') DEFAULT 'CHECKIN',
			is_mock_location    BOOLEAN DEFAULT FALSE,
			device_uptime_ms    BIGINT NULL,
			client_reported_time TIMESTAMP NULL,
			tamper_flag         BOOLEAN DEFAULT FALSE,
			liveness_passed     BOOLEAN DEFAULT TRUE,
			recorded_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE,
			FOREIGN KEY (worker_id) REFERENCES workers(id) ON DELETE CASCADE,
			INDEX idx_events_worker_time (worker_id, recorded_at),
			INDEX idx_events_mine_time (mine_id, recorded_at)
		)`,
		`CREATE TABLE IF NOT EXISTS mine_zones (
			id          INT PRIMARY KEY AUTO_INCREMENT,
			mine_id     INT NOT NULL,
			zone_name   VARCHAR(100) NOT NULL,
			zone_type   VARCHAR(100),
			latitude    DECIMAL(10,6),
			longitude   DECIMAL(10,6),
			created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS mesh_nodes (
			id              INT PRIMARY KEY AUTO_INCREMENT,
			mine_id         INT NOT NULL,
			zone_id         INT NULL,
			node_name       VARCHAR(100) NOT NULL,
			hop_sequence    INT NOT NULL,
			battery_pct     DECIMAL(5,2) DEFAULT 100.00,
			status          ENUM('ONLINE','OFFLINE') DEFAULT 'ONLINE',
			last_heartbeat  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE,
			FOREIGN KEY (zone_id) REFERENCES mine_zones(id) ON DELETE SET NULL,
			INDEX idx_mesh_mine_hop (mine_id, hop_sequence)
		)`,
		`CREATE TABLE IF NOT EXISTS sos_relay_logs (
			id                  BIGINT PRIMARY KEY AUTO_INCREMENT,
			incident_id         INT NOT NULL,
			node_id             INT NOT NULL,
			hop_number          INT NOT NULL,
			latency_ms          INT NOT NULL,
			signal_strength_pct DECIMAL(5,2) NOT NULL,
			relayed_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (incident_id) REFERENCES incidents(id) ON DELETE CASCADE,
			FOREIGN KEY (node_id) REFERENCES mesh_nodes(id) ON DELETE CASCADE,
			INDEX idx_relay_incident (incident_id)
		)`,
	}

	for _, stmt := range tables {
		_, _ = DB.Exec(stmt)
	}

	var meshCount int
	if err := DB.QueryRow("SELECT COUNT(*) FROM mesh_nodes").Scan(&meshCount); err == nil && meshCount == 0 {
		_, _ = DB.Exec(`
			INSERT INTO mine_zones (id, mine_id, zone_name, zone_type, latitude, longitude) VALUES
			(1, 1, 'Deep Seam Pit-4 / Sector B', 'UNDERGROUND_SEAM', 22.3595, 82.6892),
			(2, 1, 'Incline Haulage Roadway', 'INCLINE_HAULWAY', 22.3601, 82.6898),
			(3, 1, 'Central Ventilation Shaft', 'VENTILATION', 22.3608, 82.6905),
			(4, 2, 'Quarry Sector-3 Highwall', 'OPENCAST_FACE', 22.3167, 82.5833),
			(5, 10, 'Talcher Seam-1 Face', 'UNDERGROUND_SEAM', 20.9500, 85.2167)
		`)
		_, _ = DB.Exec(`
			INSERT INTO mesh_nodes (mine_id, zone_id, node_name, hop_sequence, battery_pct, status) VALUES
			(1, 1, 'NODE-01-SEAM-FACE',    1, 98.50, 'ONLINE'),
			(1, 2, 'NODE-02-INCLINE-WAY',   2, 94.00, 'ONLINE'),
			(1, 1, 'NODE-03-HAULAGE-XING',  3, 89.20, 'ONLINE'),
			(1, 3, 'NODE-04-VENT-SHAFT',    4, 96.00, 'ONLINE'),
			(1, 2, 'NODE-05-PIT-ENTRY',     5, 99.00, 'ONLINE'),
			(1, 3, 'NODE-06-SURFACE-GW',    6, 100.00, 'ONLINE'),
			(2, 4, 'NODE-01-BENCH-FACE',    1, 97.00, 'ONLINE'),
			(2, 4, 'NODE-02-HAUL-RAMP',     2, 91.50, 'ONLINE'),
			(2, 4, 'NODE-03-CRUSHER-FEED',  3, 88.00, 'ONLINE'),
			(2, 4, 'NODE-04-SUB-STATION',   4, 95.50, 'ONLINE'),
			(2, 4, 'NODE-05-SECURITY-GATE', 5, 98.00, 'ONLINE'),
			(2, 4, 'NODE-06-SURFACE-GW',    6, 100.00, 'ONLINE'),
			(10, 5, 'NODE-01-WORKING-FACE', 1, 96.00, 'ONLINE'),
			(10, 5, 'NODE-02-HAULAGE-DRIFT',2, 92.00, 'ONLINE'),
			(10, 5, 'NODE-03-TRANSFER-POINT',3, 87.50, 'ONLINE'),
			(10, 5, 'NODE-04-SHAFT-BOTTOM', 4, 94.00, 'ONLINE'),
			(10, 5, 'NODE-05-PITHEAD-TOWER',5, 99.00, 'ONLINE'),
			(10, 5, 'NODE-06-SURFACE-GW',   6, 100.00, 'ONLINE')
		`)
	}

	log.Println("Database schema migrations and mesh nodes verified.")
}

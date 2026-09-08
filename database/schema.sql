-- =====================================================================
-- AI-Based Smart Governance and Compliance Monitoring System
-- Coal India Limited / Ministry of Coal - SIH 2026 (SIH26024)
-- Database Schema (MySQL 8+)
-- =====================================================================

CREATE DATABASE IF NOT EXISTS coal_governance
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE coal_governance;

SET FOREIGN_KEY_CHECKS = 0;

-- 1. ROLES
CREATE TABLE roles (
    id            INT PRIMARY KEY AUTO_INCREMENT,
    role_key      VARCHAR(50) NOT NULL UNIQUE,
    role_name     VARCHAR(100) NOT NULL,
    description   VARCHAR(255),
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 2. SUBSIDIARIES
CREATE TABLE subsidiaries (
    id            INT PRIMARY KEY AUTO_INCREMENT,
    name          VARCHAR(150) NOT NULL,
    code          VARCHAR(20) NOT NULL UNIQUE,
    headquarters  VARCHAR(150),
    status        ENUM('ACTIVE','INACTIVE') DEFAULT 'ACTIVE',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- 3. USERS
CREATE TABLE users (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    full_name       VARCHAR(150) NOT NULL,
    email           VARCHAR(150) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    role_id         INT NOT NULL,
    subsidiary_id   INT NULL,
    phone           VARCHAR(20),
    status          ENUM('ACTIVE','INACTIVE','SUSPENDED') DEFAULT 'ACTIVE',
    last_login_at   DATETIME NULL,
    created_by      INT NULL,
    updated_by      INT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (role_id) REFERENCES roles(id),
    FOREIGN KEY (subsidiary_id) REFERENCES subsidiaries(id),
    INDEX idx_users_email (email),
    INDEX idx_users_role (role_id)
);

-- 4. MINES
CREATE TABLE mines (
    id                  INT PRIMARY KEY AUTO_INCREMENT,
    mine_name           VARCHAR(150) NOT NULL,
    mine_code           VARCHAR(30) NOT NULL UNIQUE,
    subsidiary_id       INT NOT NULL,
    state               VARCHAR(100),
    district            VARCHAR(100),
    latitude            DECIMAL(10,6),
    longitude           DECIMAL(10,6),
    mine_type           ENUM('OPENCAST','UNDERGROUND','MIXED') DEFAULT 'OPENCAST',
    production_capacity DECIMAL(12,2),
    manager_id          INT NULL,
    status              ENUM('ACTIVE','INACTIVE','UNDER_MAINTENANCE') DEFAULT 'ACTIVE',
    created_by          INT NULL,
    updated_by          INT NULL,
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (subsidiary_id) REFERENCES subsidiaries(id),
    FOREIGN KEY (manager_id) REFERENCES users(id),
    INDEX idx_mines_subsidiary (subsidiary_id),
    INDEX idx_mines_status (status)
);

-- 5. MINE ZONES
CREATE TABLE mine_zones (
    id          INT PRIMARY KEY AUTO_INCREMENT,
    mine_id     INT NOT NULL,
    zone_name   VARCHAR(100) NOT NULL,
    zone_type   VARCHAR(100),
    latitude    DECIMAL(10,6),
    longitude   DECIMAL(10,6),
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE
);

-- 6. DEPARTMENTS
CREATE TABLE departments (
    id          INT PRIMARY KEY AUTO_INCREMENT,
    mine_id     INT NOT NULL,
    dept_name   VARCHAR(100) NOT NULL,
    dept_head_id INT NULL,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE,
    FOREIGN KEY (dept_head_id) REFERENCES users(id)
);

-- 7. WORKERS
CREATE TABLE workers (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    mine_id         INT NOT NULL,
    worker_code     VARCHAR(30) NOT NULL UNIQUE,
    full_name       VARCHAR(150) NOT NULL,
    designation     VARCHAR(100),
    department_id   INT NULL,
    contractor_id   INT NULL,
    status          ENUM('ACTIVE','INACTIVE') DEFAULT 'ACTIVE',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE,
    FOREIGN KEY (department_id) REFERENCES departments(id)
);

-- 8. CONTRACTORS
CREATE TABLE contractors (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    mine_id         INT NOT NULL,
    company_name    VARCHAR(150) NOT NULL,
    contact_person  VARCHAR(150),
    phone           VARCHAR(20),
    email           VARCHAR(150),
    contract_type   VARCHAR(100),
    contract_start  DATE,
    contract_end    DATE,
    status          ENUM('ACTIVE','INACTIVE','BLACKLISTED') DEFAULT 'ACTIVE',
    blacklist_reason TEXT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE
);

ALTER TABLE workers ADD FOREIGN KEY (contractor_id) REFERENCES contractors(id);

-- 9. COMPLIANCE CATEGORIES
CREATE TABLE compliance_categories (
    id          INT PRIMARY KEY AUTO_INCREMENT,
    name        VARCHAR(100) NOT NULL UNIQUE,
    description VARCHAR(255)
);

-- 10. COMPLIANCE RULES
CREATE TABLE compliance_rules (
    id                  INT PRIMARY KEY AUTO_INCREMENT,
    rule_code           VARCHAR(30) NOT NULL UNIQUE,
    title               VARCHAR(200) NOT NULL,
    description         TEXT,
    category_id         INT NOT NULL,
    applicable_mine_id  INT NULL,
    frequency           ENUM('DAILY','WEEKLY','MONTHLY','QUARTERLY','ANNUAL','ONE_TIME') DEFAULT 'MONTHLY',
    severity            ENUM('LOW','MEDIUM','HIGH','CRITICAL') DEFAULT 'MEDIUM',
    responsible_dept    VARCHAR(100),
    due_period_days     INT DEFAULT 30,
    status              ENUM('ACTIVE','INACTIVE') DEFAULT 'ACTIVE',
    created_by          INT NULL,
    updated_by          INT NULL,
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (category_id) REFERENCES compliance_categories(id),
    FOREIGN KEY (applicable_mine_id) REFERENCES mines(id)
);

-- 11. INSPECTIONS
CREATE TABLE inspections (
    id                  INT PRIMARY KEY AUTO_INCREMENT,
    mine_id             INT NOT NULL,
    inspection_type     VARCHAR(100),
    inspector_id        INT NOT NULL,
    inspection_date     DATE NOT NULL,
    inspection_time     TIME,
    gps_latitude        DECIMAL(10,6),
    gps_longitude       DECIMAL(10,6),
    remarks             TEXT,
    status              ENUM('DRAFT','SUBMITTED','REVIEWED','APPROVED') DEFAULT 'DRAFT',
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id),
    FOREIGN KEY (inspector_id) REFERENCES users(id),
    INDEX idx_inspections_mine (mine_id),
    INDEX idx_inspections_date (inspection_date)
);

-- 12. INSPECTION ITEMS
CREATE TABLE inspection_items (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    inspection_id   INT NOT NULL,
    checklist_item  VARCHAR(255) NOT NULL,
    result          ENUM('PASS','FAIL','NA') DEFAULT 'NA',
    remarks         VARCHAR(255),
    FOREIGN KEY (inspection_id) REFERENCES inspections(id) ON DELETE CASCADE
);

-- 13. OBSERVATIONS
CREATE TABLE observations (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    inspection_id   INT NOT NULL,
    category_id     INT NULL,
    observation     TEXT NOT NULL,
    severity        ENUM('LOW','MEDIUM','HIGH','CRITICAL') DEFAULT 'LOW',
    evidence_path   VARCHAR(255),
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (inspection_id) REFERENCES inspections(id) ON DELETE CASCADE,
    FOREIGN KEY (category_id) REFERENCES compliance_categories(id)
);

-- 14. VIOLATIONS
CREATE TABLE violations (
    id                  INT PRIMARY KEY AUTO_INCREMENT,
    violation_code      VARCHAR(30) NOT NULL UNIQUE,
    mine_id             INT NOT NULL,
    inspection_id       INT NULL,
    category_id         INT NOT NULL,
    description         TEXT NOT NULL,
    severity            ENUM('LOW','MEDIUM','HIGH','CRITICAL') DEFAULT 'MEDIUM',
    evidence_path       VARCHAR(255),
    reported_by         INT NOT NULL,
    responsible_person  INT NULL,
    deadline            DATE,
    status              ENUM('OPEN','IN_PROGRESS','RESOLVED','VERIFIED','CLOSED','OVERDUE') DEFAULT 'OPEN',
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id),
    FOREIGN KEY (inspection_id) REFERENCES inspections(id),
    FOREIGN KEY (category_id) REFERENCES compliance_categories(id),
    FOREIGN KEY (reported_by) REFERENCES users(id),
    FOREIGN KEY (responsible_person) REFERENCES users(id),
    INDEX idx_violations_mine (mine_id),
    INDEX idx_violations_status (status)
);

-- 15. CORRECTIVE ACTIONS
CREATE TABLE corrective_actions (
    id                  INT PRIMARY KEY AUTO_INCREMENT,
    violation_id        INT NOT NULL,
    assigned_to         INT NOT NULL,
    action_description  TEXT NOT NULL,
    deadline            DATE NOT NULL,
    submitted_at        DATETIME NULL,
    verified_by         INT NULL,
    verified_at         DATETIME NULL,
    escalation_level    INT DEFAULT 0,
    status              ENUM('ASSIGNED','SUBMITTED','VERIFIED','CLOSED','OVERDUE') DEFAULT 'ASSIGNED',
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (violation_id) REFERENCES violations(id),
    FOREIGN KEY (assigned_to) REFERENCES users(id),
    FOREIGN KEY (verified_by) REFERENCES users(id)
);

-- 16. INCIDENTS
CREATE TABLE incidents (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    mine_id         INT NOT NULL,
    incident_type   VARCHAR(100),
    description     TEXT,
    severity        ENUM('LOW','MEDIUM','HIGH','CRITICAL') DEFAULT 'MEDIUM',
    reported_by     INT NOT NULL,
    incident_date   DATETIME NOT NULL,
    status          ENUM('OPEN','UNDER_REVIEW','CLOSED') DEFAULT 'OPEN',
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id),
    FOREIGN KEY (reported_by) REFERENCES users(id)
);

-- 17. OPERATIONAL DATA
CREATE TABLE operational_data (
    id                  BIGINT PRIMARY KEY AUTO_INCREMENT,
    mine_id             INT NOT NULL,
    record_date         DATE NOT NULL,
    production_tonnes   DECIMAL(12,2),
    expected_production DECIMAL(12,2),
    equipment_health_pct DECIMAL(5,2),
    attendance_pct      DECIMAL(5,2),
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id),
    INDEX idx_opdata_mine_date (mine_id, record_date)
);

-- 18. ENVIRONMENTAL DATA
CREATE TABLE environmental_data (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    mine_id         INT NOT NULL,
    record_date     DATE NOT NULL,
    aqi             DECIMAL(6,2),
    water_quality_index DECIMAL(6,2),
    noise_level_db  DECIMAL(6,2),
    dust_level      DECIMAL(6,2),
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id)
);

-- 19. ATTENDANCE
CREATE TABLE attendance (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    mine_id         INT NOT NULL,
    worker_id       INT NULL,
    record_date     DATE NOT NULL,
    status          ENUM('PRESENT','ABSENT','LEAVE','HALF_DAY') DEFAULT 'PRESENT',
    shift           VARCHAR(20) DEFAULT 'GENERAL',
    overtime_hours  DECIMAL(4,2) DEFAULT 0.00,
    present_count   INT NULL,
    total_count     INT NULL,
    marked_by       INT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id),
    FOREIGN KEY (worker_id) REFERENCES workers(id),
    FOREIGN KEY (marked_by) REFERENCES users(id),
    INDEX idx_attendance_mine_date (mine_id, record_date),
    INDEX idx_attendance_worker (worker_id)
);

-- 20. DOCUMENTS
CREATE TABLE documents (
    id                  INT PRIMARY KEY AUTO_INCREMENT,
    mine_id             INT NULL,
    contractor_id       INT NULL,
    document_type       VARCHAR(100),
    file_path           VARCHAR(255) NOT NULL,
    certificate_number  VARCHAR(100),
    issue_date          DATE,
    expiry_date         DATE,
    ocr_raw_text        TEXT,
    mine_code           VARCHAR(50) NULL,
    inspector_name      VARCHAR(100) NULL,
    inspection_date     DATE NULL,
    compliance_status   VARCHAR(50) DEFAULT 'COMPLIANT',
    violation_details   TEXT NULL,
    risk_level          VARCHAR(30) DEFAULT 'LOW',
    corrective_action   TEXT NULL,
    due_date            DATE NULL,
    regulatory_reference VARCHAR(255) NULL,
    ocr_data_json       JSON NULL,
    workflow_status     VARCHAR(50) DEFAULT 'PENDING_REVIEW',
    uploaded_by         INT NOT NULL,
    reviewed_by         INT NULL,
    reviewed_at         DATETIME NULL,
    approved_by         INT NULL,
    approved_at         DATETIME NULL,
    verified_by         INT NULL,
    verified_at         DATETIME NULL,
    status              ENUM('VALID','EXPIRING_SOON','EXPIRED') DEFAULT 'VALID',
    created_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id),
    FOREIGN KEY (contractor_id) REFERENCES contractors(id),
    FOREIGN KEY (uploaded_by) REFERENCES users(id),
    FOREIGN KEY (reviewed_by) REFERENCES users(id),
    FOREIGN KEY (approved_by) REFERENCES users(id),
    FOREIGN KEY (verified_by) REFERENCES users(id),
    INDEX idx_documents_contractor (contractor_id)
);

-- 21. RISK SCORES
CREATE TABLE risk_scores (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    mine_id         INT NOT NULL,
    score           DECIMAL(5,2) NOT NULL,
    classification  ENUM('LOW','MEDIUM','HIGH','CRITICAL') NOT NULL,
    factors_json    JSON,
    computed_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id),
    INDEX idx_risk_mine (mine_id)
);

-- 22. ANOMALIES
CREATE TABLE anomalies (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    mine_id         INT NOT NULL,
    anomaly_type    VARCHAR(100),
    description     TEXT,
    detected_value  DECIMAL(12,2),
    expected_value  DECIMAL(12,2),
    severity        ENUM('LOW','MEDIUM','HIGH','CRITICAL') DEFAULT 'MEDIUM',
    status          ENUM('NEW','ACKNOWLEDGED','RESOLVED') DEFAULT 'NEW',
    detected_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (mine_id) REFERENCES mines(id)
);

-- 23. NOTIFICATIONS
CREATE TABLE notifications (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    recipient_id    INT NOT NULL,
    title           VARCHAR(200) NOT NULL,
    message         TEXT NOT NULL,
    severity        ENUM('INFO','WARNING','CRITICAL') DEFAULT 'INFO',
    type            VARCHAR(50),
    is_read         BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (recipient_id) REFERENCES users(id),
    INDEX idx_notif_recipient (recipient_id, is_read)
);

-- 24. AUDIT LOGS (Hash-chained for tamper evidence)
CREATE TABLE audit_logs (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id         INT NULL,
    action          VARCHAR(100) NOT NULL,
    module          VARCHAR(100),
    record_id       VARCHAR(50),
    details         JSON,
    ip_address      VARCHAR(50),
    prev_hash       VARCHAR(64) NULL,
    hash            VARCHAR(64) NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    INDEX idx_audit_user (user_id),
    INDEX idx_audit_module (module),
    INDEX idx_audit_created (created_at)
);

-- 25. GRIEVANCES
CREATE TABLE grievances (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    worker_id       INT NULL,
    mine_id         INT NOT NULL,
    category        VARCHAR(100) NOT NULL,
    description     TEXT NOT NULL,
    status          ENUM('SUBMITTED','IN_REVIEW','RESOLVED','ESCALATED','CLOSED') DEFAULT 'SUBMITTED',
    assigned_to     INT NULL,
    resolution_notes TEXT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (worker_id) REFERENCES workers(id) ON DELETE SET NULL,
    FOREIGN KEY (mine_id) REFERENCES mines(id) ON DELETE CASCADE,
    FOREIGN KEY (assigned_to) REFERENCES users(id) ON DELETE SET NULL,
    INDEX idx_grievances_mine (mine_id),
    INDEX idx_grievances_status (status)
);

-- 26. REPORTS
CREATE TABLE reports (
    id              INT PRIMARY KEY AUTO_INCREMENT,
    report_type     VARCHAR(100) NOT NULL,
    generated_by    INT NOT NULL,
    mine_id         INT NULL,
    file_path       VARCHAR(255),
    format          ENUM('PDF','CSV') DEFAULT 'PDF',
    parameters_json JSON,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (generated_by) REFERENCES users(id),
    FOREIGN KEY (mine_id) REFERENCES mines(id)
);

-- 27. AI INSPECTION ANALYSES
CREATE TABLE ai_inspection_analyses (
    id                 INT PRIMARY KEY AUTO_INCREMENT,
    inspection_id      INT NOT NULL,
    category           VARCHAR(100),
    severity           ENUM('LOW','MEDIUM','HIGH','CRITICAL') DEFAULT 'LOW',
    risk_level         ENUM('LOW','MEDIUM','HIGH','CRITICAL') DEFAULT 'LOW',
    risk_score         INT NOT NULL,
    summary            TEXT,
    reasoning          TEXT,
    recommended_action TEXT,
    recurring_issue    BOOLEAN DEFAULT FALSE,
    urgency            VARCHAR(50),
    confidence         DECIMAL(5,2),
    model_name         VARCHAR(100),
    created_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (inspection_id) REFERENCES inspections(id) ON DELETE CASCADE
);

SET FOREIGN_KEY_CHECKS = 1;

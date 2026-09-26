package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"coal-governance-backend/config"
	"coal-governance-backend/database"
	"coal-governance-backend/utils"

	"github.com/robfig/cron/v3"
)

var (
	cronRunner     *cron.Cron
	cronMutex      sync.Mutex
	isEscalatingMu sync.Mutex
)

// SLAEscalationResult summarizes the count of records escalated during a cycle.
type SLAEscalationResult struct {
	ViolationsL2Count int `json:"violations_l2_count"`
	ViolationsL3Count int `json:"violations_l3_count"`
	GrievancesL2Count int `json:"grievances_l2_count"`
	GrievancesL3Count int `json:"grievances_l3_count"`
	TotalEscalated    int `json:"total_escalated"`
}

// StartSLAEscalationCron initializes and starts the background SLA cron runner.
func StartSLAEscalationCron(cfg *config.Config) {
	cronMutex.Lock()
	defer cronMutex.Unlock()

	if cronRunner != nil {
		cronRunner.Stop()
	}

	cronRunner = cron.New()
	schedule := cfg.SLACronSchedule
	if schedule == "" {
		schedule = "@hourly"
	}

	_, err := cronRunner.AddFunc(schedule, func() {
		log.Printf("[SLA Cron] Running scheduled SLA escalation check (%s)...", schedule)
		result := CheckAndEscalateSLAs()
		log.Printf("[SLA Cron] SLA escalation completed: %+v", result)
	})

	if err != nil {
		log.Printf("[SLA Cron] Error scheduling cron expression '%s': %v. Falling back to @hourly.", schedule, err)
		_, _ = cronRunner.AddFunc("@hourly", func() {
			CheckAndEscalateSLAs()
		})
	}

	cronRunner.Start()
	log.Printf("[SLA Cron] Background SLA cron scheduler started with schedule: %s", schedule)

	// Trigger an immediate check on startup asynchronously
	go func() {
		time.Sleep(2 * time.Second)
		log.Println("[SLA Cron] Performing initial SLA escalation audit on startup...")
		CheckAndEscalateSLAs()
	}()
}

// CheckAndEscalateSLAs evaluates all open violations and grievances for SLA breaches and escalates them.
func CheckAndEscalateSLAs() SLAEscalationResult {
	isEscalatingMu.Lock()
	defer isEscalatingMu.Unlock()

	result := SLAEscalationResult{}

	// 1. Violations Level 1 -> 2
	result.ViolationsL2Count = escalateViolationsL1toL2()

	// 2. Violations Level 2 -> 3
	result.ViolationsL3Count = escalateViolationsL2toL3()

	// 3. Grievances Level 1 -> 2
	result.GrievancesL2Count = escalateGrievancesL1toL2()

	// 4. Grievances Level 2 -> 3
	result.GrievancesL3Count = escalateGrievancesL2toL3()

	result.TotalEscalated = result.ViolationsL2Count + result.ViolationsL3Count + result.GrievancesL2Count + result.GrievancesL3Count
	return result
}

// ------------------------------------------------------------------------------------------------
// VIOLATIONS ESCALATION
// ------------------------------------------------------------------------------------------------

type violationRow struct {
	ID             int
	ViolationCode  string
	MineID         int
	CategoryID     int
	Description    string
	Severity       string
	SLAHours       int
	CreatedAt      time.Time
	EscalatedAt    *time.Time
}

func escalateViolationsL1toL2() int {
	query := `
		SELECT id, violation_code, mine_id, category_id, description, severity, sla_hours, created_at
		FROM violations
		WHERE status IN ('OPEN', 'IN_PROGRESS')
		  AND escalation_level = 1
		  AND NOW() > DATE_ADD(created_at, INTERVAL sla_hours HOUR)`

	rows, err := database.DB.Query(query)
	if err != nil {
		log.Printf("[SLA Escalation] Error querying L1 violations: %v", err)
		return 0
	}
	defer rows.Close()

	var toEscalate []violationRow
	for rows.Next() {
		var v violationRow
		if err := rows.Scan(&v.ID, &v.ViolationCode, &v.MineID, &v.CategoryID, &v.Description, &v.Severity, &v.SLAHours, &v.CreatedAt); err == nil {
			toEscalate = append(toEscalate, v)
		}
	}

	count := 0
	for _, v := range toEscalate {
		// Update DB
		_, err := database.DB.Exec(`
			UPDATE violations 
			SET escalation_level = 2, escalated_at = NOW(), status = 'OVERDUE'
			WHERE id = ?`, v.ID)
		if err != nil {
			log.Printf("[SLA Escalation] Error updating violation %s to L2: %v", v.ViolationCode, err)
			continue
		}
		count++

		// In-app notifications to Mine Manager & Corporate Manager
		title := fmt.Sprintf("SLA BREACH (Level 2): Violation %s Overdue", v.ViolationCode)
		message := fmt.Sprintf("Violation %s (%s) has breached its %d-hour SLA window and is escalated to Level 2 (Subsidiary HQ).", v.ViolationCode, v.Severity, v.SLAHours)
		notifyRoles([]string{"CORPORATE_MANAGER", "MINE_MANAGER"}, v.MineID, title, message, "CRITICAL", "ESCALATION")

		// Audit Log
		utils.LogAudit(0, "VIOLATION_SLA_ESCALATED_L2", "VIOLATIONS", strconv.Itoa(v.ID), map[string]interface{}{
			"violation_code":   v.ViolationCode,
			"sla_hours":        v.SLAHours,
			"escalation_level": 2,
			"mine_id":          v.MineID,
			"severity":         v.Severity,
			"reason":           fmt.Sprintf("SLA window of %d hours exceeded", v.SLAHours),
		}, "127.0.0.1")

		log.Printf("[SLA Escalation] Violation %s escalated to Level 2 (L2)", v.ViolationCode)
	}

	return count
}

func escalateViolationsL2toL3() int {
	query := `
		SELECT id, violation_code, mine_id, category_id, description, severity, sla_hours, escalated_at, created_at
		FROM violations
		WHERE status NOT IN ('RESOLVED', 'VERIFIED', 'CLOSED')
		  AND escalation_level = 2
		  AND (severity = 'CRITICAL' OR sla_hours = 2)
		  AND escalated_at IS NOT NULL
		  AND NOW() >= DATE_ADD(escalated_at, INTERVAL 7 DAY)`

	rows, err := database.DB.Query(query)
	if err != nil {
		log.Printf("[SLA Escalation] Error querying L2->L3 violations: %v", err)
		return 0
	}
	defer rows.Close()

	var toEscalate []violationRow
	for rows.Next() {
		var v violationRow
		if err := rows.Scan(&v.ID, &v.ViolationCode, &v.MineID, &v.CategoryID, &v.Description, &v.Severity, &v.SLAHours, &v.EscalatedAt, &v.CreatedAt); err == nil {
			toEscalate = append(toEscalate, v)
		}
	}

	count := 0
	for _, v := range toEscalate {
		// Update DB
		_, err := database.DB.Exec(`
			UPDATE violations 
			SET escalation_level = 3
			WHERE id = ?`, v.ID)
		if err != nil {
			log.Printf("[SLA Escalation] Error updating violation %s to L3: %v", v.ViolationCode, err)
			continue
		}
		count++

		// In-app notifications to Regulatory Officer & Super Admin
		title := fmt.Sprintf("LEVEL 3 REGULATORY ESCALATION: Unresolved Critical Breach %s", v.ViolationCode)
		message := fmt.Sprintf("Severe compliance breach: Critical violation %s has remained unresolved for >7 days post-escalation. Escalated to Regulatory Authority / DGMS.", v.ViolationCode)
		notifyRoles([]string{"REGULATORY_OFFICER", "SUPER_ADMIN"}, 0, title, message, "CRITICAL", "REGULATORY_ESCALATION")

		// Create Report Record in reports table
		reportParams, _ := json.Marshal(map[string]interface{}{
			"violation_id":     v.ID,
			"violation_code":   v.ViolationCode,
			"description":      v.Description,
			"severity":         v.Severity,
			"sla_hours":        v.SLAHours,
			"escalation_level": 3,
			"days_unresolved":  7,
			"notice_type":      "DIRECTORATE_GENERAL_MINES_SAFETY_ALERT",
		})
		systemAdminID := getSystemAdminUserID()

		_, _ = database.DB.Exec(`
			INSERT INTO reports (report_type, generated_by, mine_id, file_path, format, parameters_json)
			VALUES (?, ?, ?, ?, 'PDF', ?)`,
			"SEVERE_COMPLIANCE_BREACH", systemAdminID, v.MineID, fmt.Sprintf("reports/sla_breach_v_%d.json", v.ID), string(reportParams))

		// Audit Log
		utils.LogAudit(0, "VIOLATION_SLA_ESCALATED_L3_REGULATORY", "VIOLATIONS", strconv.Itoa(v.ID), map[string]interface{}{
			"violation_code":   v.ViolationCode,
			"severity":         v.Severity,
			"escalation_level": 3,
			"mine_id":          v.MineID,
			"regulatory_alert": true,
			"reason":           "Critical violation unresolved for 7+ days after L2 escalation",
		}, "127.0.0.1")

		log.Printf("[SLA Escalation] Violation %s escalated to Regulatory Level 3 (L3)", v.ViolationCode)
	}

	return count
}

// ------------------------------------------------------------------------------------------------
// GRIEVANCES ESCALATION
// ------------------------------------------------------------------------------------------------

type grievanceRow struct {
	ID          int
	MineID      int
	Category    string
	Description string
	SLAHours    int
	CreatedAt   time.Time
	EscalatedAt *time.Time
}

func escalateGrievancesL1toL2() int {
	query := `
		SELECT id, mine_id, category, description, sla_hours, created_at
		FROM grievances
		WHERE status IN ('SUBMITTED', 'IN_REVIEW')
		  AND escalation_level = 1
		  AND NOW() > DATE_ADD(created_at, INTERVAL sla_hours HOUR)`

	rows, err := database.DB.Query(query)
	if err != nil {
		log.Printf("[SLA Escalation] Error querying L1 grievances: %v", err)
		return 0
	}
	defer rows.Close()

	var toEscalate []grievanceRow
	for rows.Next() {
		var g grievanceRow
		if err := rows.Scan(&g.ID, &g.MineID, &g.Category, &g.Description, &g.SLAHours, &g.CreatedAt); err == nil {
			toEscalate = append(toEscalate, g)
		}
	}

	count := 0
	for _, g := range toEscalate {
		// Update DB
		_, err := database.DB.Exec(`
			UPDATE grievances 
			SET escalation_level = 2, escalated_at = NOW(), status = 'ESCALATED'
			WHERE id = ?`, g.ID)
		if err != nil {
			log.Printf("[SLA Escalation] Error updating grievance #%d to L2: %v", g.ID, err)
			continue
		}
		count++

		// In-app notifications
		title := fmt.Sprintf("SLA BREACH (Level 2): Grievance #%d Escalated", g.ID)
		message := fmt.Sprintf("Worker grievance #%d (%s) has exceeded its %d-hour SLA window and is escalated to Level 2 (Headquarters).", g.ID, g.Category, g.SLAHours)
		notifyRoles([]string{"CORPORATE_MANAGER", "MINE_MANAGER"}, g.MineID, title, message, "CRITICAL", "GRIEVANCE_ESCALATION")

		// Audit Log
		utils.LogAudit(0, "GRIEVANCE_SLA_ESCALATED_L2", "GRIEVANCES", strconv.Itoa(g.ID), map[string]interface{}{
			"grievance_id":     g.ID,
			"category":         g.Category,
			"sla_hours":        g.SLAHours,
			"escalation_level": 2,
			"mine_id":          g.MineID,
			"reason":           fmt.Sprintf("Grievance SLA window of %d hours exceeded", g.SLAHours),
		}, "127.0.0.1")

		log.Printf("[SLA Escalation] Grievance #%d escalated to Level 2 (L2)", g.ID)
	}

	return count
}

func escalateGrievancesL2toL3() int {
	query := `
		SELECT id, mine_id, category, description, sla_hours, escalated_at, created_at
		FROM grievances
		WHERE status NOT IN ('RESOLVED', 'CLOSED')
		  AND escalation_level = 2
		  AND (category = 'Safety' OR sla_hours = 2)
		  AND escalated_at IS NOT NULL
		  AND NOW() >= DATE_ADD(escalated_at, INTERVAL 7 DAY)`

	rows, err := database.DB.Query(query)
	if err != nil {
		log.Printf("[SLA Escalation] Error querying L2->L3 grievances: %v", err)
		return 0
	}
	defer rows.Close()

	var toEscalate []grievanceRow
	for rows.Next() {
		var g grievanceRow
		if err := rows.Scan(&g.ID, &g.MineID, &g.Category, &g.Description, &g.SLAHours, &g.EscalatedAt, &g.CreatedAt); err == nil {
			toEscalate = append(toEscalate, g)
		}
	}

	count := 0
	for _, g := range toEscalate {
		// Update DB
		_, err := database.DB.Exec(`
			UPDATE grievances 
			SET escalation_level = 3
			WHERE id = ?`, g.ID)
		if err != nil {
			log.Printf("[SLA Escalation] Error updating grievance #%d to L3: %v", g.ID, err)
			continue
		}
		count++

		// In-app notifications
		title := fmt.Sprintf("LEVEL 3 REGULATORY ESCALATION: Unresolved Safety Grievance #%d", g.ID)
		message := fmt.Sprintf("Safety Grievance #%d remained unresolved for >7 days post-escalation. Escalated to Regulatory Authority / DGMS.", g.ID)
		notifyRoles([]string{"REGULATORY_OFFICER", "SUPER_ADMIN"}, 0, title, message, "CRITICAL", "REGULATORY_ESCALATION")

		// Create Report Record
		reportParams, _ := json.Marshal(map[string]interface{}{
			"grievance_id":     g.ID,
			"category":         g.Category,
			"description":      g.Description,
			"sla_hours":        g.SLAHours,
			"escalation_level": 3,
			"days_unresolved":  7,
			"notice_type":      "UNRESOLVED_SAFETY_GRIEVANCE_BREACH",
		})
		systemAdminID := getSystemAdminUserID()

		_, _ = database.DB.Exec(`
			INSERT INTO reports (report_type, generated_by, mine_id, file_path, format, parameters_json)
			VALUES (?, ?, ?, ?, 'PDF', ?)`,
			"SEVERE_GRIEVANCE_BREACH", systemAdminID, g.MineID, fmt.Sprintf("reports/sla_grievance_breach_%d.json", g.ID), string(reportParams))

		// Audit Log
		utils.LogAudit(0, "GRIEVANCE_SLA_ESCALATED_L3_REGULATORY", "GRIEVANCES", strconv.Itoa(g.ID), map[string]interface{}{
			"grievance_id":     g.ID,
			"category":         g.Category,
			"escalation_level": 3,
			"mine_id":          g.MineID,
			"regulatory_alert": true,
			"reason":           "Critical safety grievance unresolved for 7+ days after L2 escalation",
		}, "127.0.0.1")

		log.Printf("[SLA Escalation] Grievance #%d escalated to Regulatory Level 3 (L3)", g.ID)
	}

	return count
}

// ------------------------------------------------------------------------------------------------
// HELPER FUNCTIONS
// ------------------------------------------------------------------------------------------------

func notifyRoles(roleKeys []string, mineID int, title, message, severity, notifType string) {
	if len(roleKeys) == 0 {
		return
	}

	for _, roleKey := range roleKeys {
		var query string
		var args []interface{}

		if mineID > 0 && roleKey == "MINE_MANAGER" {
			query = `
				SELECT u.id FROM users u
				JOIN roles r ON r.id = u.role_id
				WHERE r.role_key = ? AND (u.mine_id = ? OR u.mine_id IS NULL) AND u.status = 'ACTIVE'`
			args = []interface{}{roleKey, mineID}
		} else {
			query = `
				SELECT u.id FROM users u
				JOIN roles r ON r.id = u.role_id
				WHERE r.role_key = ? AND u.status = 'ACTIVE'`
			args = []interface{}{roleKey}
		}

		rows, err := database.DB.Query(query, args...)
		if err != nil {
			log.Printf("[SLA Notification] Query error for role %s: %v", roleKey, err)
			continue
		}

		var uids []int
		for rows.Next() {
			var uid int
			if err := rows.Scan(&uid); err == nil {
				uids = append(uids, uid)
			}
		}
		rows.Close()

		for _, uid := range uids {
			_, _ = database.DB.Exec(`
				INSERT INTO notifications (recipient_id, title, message, severity, type, is_read)
				VALUES (?, ?, ?, ?, ?, FALSE)`,
				uid, title, message, severity, notifType)
		}
	}
}

func getSystemAdminUserID() int {
	var adminID int
	err := database.DB.QueryRow(`
		SELECT u.id FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE r.role_key = 'SUPER_ADMIN' AND u.status = 'ACTIVE'
		ORDER BY u.id ASC LIMIT 1`).Scan(&adminID)
	if err != nil || adminID == 0 {
		return 1
	}
	return adminID
}

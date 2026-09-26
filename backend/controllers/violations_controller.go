package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"coal-governance-backend/config"
	"coal-governance-backend/database"
	"coal-governance-backend/middleware"
	"coal-governance-backend/models"
	"coal-governance-backend/services"
	"coal-governance-backend/utils"
)

type ViolationsController struct {
	Cfg *config.Config
}

func NewViolationsController(cfg *config.Config) *ViolationsController {
	return &ViolationsController{Cfg: cfg}
}

// ListViolations lists violations with optional filters.
func (vc *ViolationsController) ListViolations(c *gin.Context) {
	query := `
		SELECT v.id, v.violation_code, v.mine_id, m.mine_name, v.inspection_id, v.category_id, cc.name AS category_name,
		       v.description, v.severity, v.evidence_path, v.reported_by, u1.full_name AS reported_by_name,
		       v.responsible_person, u2.full_name AS responsible_name, v.deadline, v.status,
		       COALESCE(v.escalation_level, 1), COALESCE(v.sla_hours, 48), v.escalated_at, v.created_at
		FROM violations v
		JOIN mines m ON m.id = v.mine_id
		JOIN compliance_categories cc ON cc.id = v.category_id
		JOIN users u1 ON u1.id = v.reported_by
		LEFT JOIN users u2 ON u2.id = v.responsible_person
		WHERE 1=1`
	args := []interface{}{}

	if mineID := c.Query("mine_id"); mineID != "" {
		query += " AND v.mine_id = ?"
		args = append(args, mineID)
	}
	if status := c.Query("status"); status != "" {
		query += " AND v.status = ?"
		args = append(args, status)
	}
	if severity := c.Query("severity"); severity != "" {
		query += " AND v.severity = ?"
		args = append(args, severity)
	}
	if categoryID := c.Query("category_id"); categoryID != "" {
		query += " AND v.category_id = ?"
		args = append(args, categoryID)
	}

	query += " ORDER BY v.created_at DESC"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to fetch violations", err.Error())
		return
	}
	defer rows.Close()

	violations := []models.Violation{}
	for rows.Next() {
		var v models.Violation
		var deadlineVal []uint8
		var insID sql.NullInt64
		var respID sql.NullInt64
		var respName sql.NullString
		var evPath sql.NullString
		var escLevel, slaHours int
		var escAt sql.NullTime

		err := rows.Scan(&v.ID, &v.ViolationCode, &v.MineID, &v.MineName, &insID, &v.CategoryID, &v.CategoryName,
			&v.Description, &v.Severity, &evPath, &v.ReportedBy, &v.ReportedByName,
			&respID, &respName, &deadlineVal, &v.Status,
			&escLevel, &slaHours, &escAt, &v.CreatedAt)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to parse violations", err.Error())
			return
		}

		v.EscalationLevel = escLevel
		v.SLAHours = slaHours
		if escAt.Valid {
			v.EscalatedAt = &escAt.Time
		}

		if deadlineVal != nil {
			v.Deadline = string(deadlineVal)
		}
		if insID.Valid {
			id := int(insID.Int64)
			v.InspectionID = &id
		}
		if respID.Valid {
			id := int(respID.Int64)
			v.ResponsiblePerson = &id
		}
		if respName.Valid {
			v.ResponsibleName = respName.String
		}
		if evPath.Valid {
			v.EvidencePath = evPath.String
		}

		violations = append(violations, v)
	}

	utils.Success(c, http.StatusOK, "Violations fetched successfully", violations)
}

// GetViolation gets details of a single violation.
func (vc *ViolationsController) GetViolation(c *gin.Context) {
	id := c.Param("id")

	var v models.Violation
	var deadlineVal []uint8
	var insID sql.NullInt64
	var respID sql.NullInt64
	var respName sql.NullString
	var evPath sql.NullString
	var escLevel, slaHours int
	var escAt sql.NullTime

	err := database.DB.QueryRow(`
		SELECT v.id, v.violation_code, v.mine_id, m.mine_name, v.inspection_id, v.category_id, cc.name AS category_name,
		       v.description, v.severity, v.evidence_path, v.reported_by, u1.full_name AS reported_by_name,
		       v.responsible_person, u2.full_name AS responsible_name, v.deadline, v.status,
		       COALESCE(v.escalation_level, 1), COALESCE(v.sla_hours, 48), v.escalated_at, v.created_at
		FROM violations v
		JOIN mines m ON m.id = v.mine_id
		JOIN compliance_categories cc ON cc.id = v.category_id
		JOIN users u1 ON u1.id = v.reported_by
		LEFT JOIN users u2 ON u2.id = v.responsible_person
		WHERE v.id = ?`, id).Scan(&v.ID, &v.ViolationCode, &v.MineID, &v.MineName, &insID, &v.CategoryID, &v.CategoryName,
		&v.Description, &v.Severity, &evPath, &v.ReportedBy, &v.ReportedByName,
		&respID, &respName, &deadlineVal, &v.Status,
		&escLevel, &slaHours, &escAt, &v.CreatedAt)

	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Violation not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	v.EscalationLevel = escLevel
	v.SLAHours = slaHours
	if escAt.Valid {
		v.EscalatedAt = &escAt.Time
	}

	if deadlineVal != nil {
		v.Deadline = string(deadlineVal)
	}
	if insID.Valid {
		id := int(insID.Int64)
		v.InspectionID = &id
	}
	if respID.Valid {
		id := int(respID.Int64)
		v.ResponsiblePerson = &id
	}
	if respName.Valid {
		v.ResponsibleName = respName.String
	}
	if evPath.Valid {
		v.EvidencePath = evPath.String
	}

	utils.Success(c, http.StatusOK, "Violation details fetched", v)
}

type manualViolationRequest struct {
	MineID       int    `json:"mine_id" binding:"required"`
	CategoryID   int    `json:"category_id" binding:"required"`
	InspectionID *int   `json:"inspection_id"`
	Description  string `json:"description" binding:"required"`
	Severity     string `json:"severity" binding:"required"` // LOW, MEDIUM, HIGH, CRITICAL
	Deadline     string `json:"deadline"`                   // YYYY-MM-DD
}

// CreateViolation handles direct manual logging of a compliance breach.
func (vc *ViolationsController) CreateViolation(c *gin.Context) {
	var req manualViolationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid payload", err.Error())
		return
	}

	userID, _ := c.Get(middleware.CtxUserID)

	// Determine violation code
	year := time.Now().Format("2006")
	var seq int
	_ = database.DB.QueryRow(`SELECT COUNT(*) + 1 FROM violations`).Scan(&seq)
	violationCode := fmt.Sprintf("VIO-%s-%04d", year, seq)

	// Determine SLA hours dynamically via AI classifier / deterministic fallback
	var aiURL string
	if vc.Cfg != nil {
		aiURL = vc.Cfg.AIServiceURL
	}
	classification, slaHours := services.ClassifySLAWithAI(aiURL, req.Description)

	// If manually marked CRITICAL, ensure SLA is 2 hours
	if req.Severity == "CRITICAL" && slaHours > 2 {
		slaHours = 2
		classification = "CRITICAL_HAZARD"
	}

	// Handle default deadline if not provided
	if req.Deadline == "" {
		now := time.Now()
		var deadline time.Time
		switch slaHours {
		case 2:
			deadline = now.Add(2 * time.Hour)
		case 12:
			deadline = now.Add(12 * time.Hour)
		default:
			switch req.Severity {
			case "CRITICAL":
				deadline = now.AddDate(0, 0, 1)
			case "HIGH":
				deadline = now.AddDate(0, 0, 3)
			case "MEDIUM":
				deadline = now.AddDate(0, 0, 7)
			default:
				deadline = now.AddDate(0, 0, 14)
			}
		}
		req.Deadline = deadline.Format("2006-01-02")
	}

	res, err := database.DB.Exec(`
		INSERT INTO violations (violation_code, mine_id, category_id, inspection_id, description, severity, reported_by, deadline, status, escalation_level, sla_hours)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'OPEN', 1, ?)`,
		violationCode, req.MineID, req.CategoryID, req.InspectionID, req.Description, req.Severity, userID, req.Deadline, slaHours)

	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to record manual violation", err.Error())
		return
	}

	newID, _ := res.LastInsertId()
	utils.LogAudit(userID.(int), "VIOLATION_MANUALLY_CREATED", "VIOLATIONS", violationCode,
		map[string]interface{}{"mine_id": req.MineID, "severity": req.Severity, "sla_hours": slaHours, "classification": classification}, c.ClientIP())

	utils.Success(c, http.StatusCreated, "Violation reported successfully", gin.H{
		"id":             newID,
		"violation_code": violationCode,
		"sla_hours":      slaHours,
		"classification": classification,
	})
}

type updateViolationRequest struct {
	ResponsiblePerson *int   `json:"responsible_person"`
	Deadline          string `json:"deadline"`
	Status            string `json:"status"` // OPEN, IN_PROGRESS, RESOLVED, VERIFIED, CLOSED, OVERDUE
}

// UpdateViolation handles updating assignee, deadline, or status.
func (vc *ViolationsController) UpdateViolation(c *gin.Context) {
	id := c.Param("id")
	var req updateViolationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid payload", err.Error())
		return
	}

	userID, _ := c.Get(middleware.CtxUserID)

	// Fetch current violation record
	var currentStatus string
	var currentCode string
	err := database.DB.QueryRow(`SELECT status, violation_code FROM violations WHERE id = ?`, id).Scan(&currentStatus, &currentCode)
	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Violation not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// Build update statement dynamically
	query := "UPDATE violations SET updated_at = NOW()"
	args := []interface{}{}

	if req.ResponsiblePerson != nil {
		query += ", responsible_person = ?"
		args = append(args, *req.ResponsiblePerson)
		// If OPEN and assigning someone, transition to IN_PROGRESS
		if req.Status == "" && currentStatus == "OPEN" {
			req.Status = "IN_PROGRESS"
		}
	}
	if req.Deadline != "" {
		query += ", deadline = ?"
		args = append(args, req.Deadline)
	}
	if req.Status != "" {
		query += ", status = ?"
		args = append(args, req.Status)
	}

	query += " WHERE id = ?"
	args = append(args, id)

	_, err = database.DB.Exec(query, args...)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to update violation", err.Error())
		return
	}

	utils.LogAudit(userID.(int), "VIOLATION_UPDATED", "VIOLATIONS", currentCode,
		map[string]interface{}{"status": req.Status, "responsible_person": req.ResponsiblePerson}, c.ClientIP())

	utils.Success(c, http.StatusOK, "Violation updated successfully", nil)
}

type assignViolationRequest struct {
	AssignedTo        int    `json:"assigned_to" binding:"required"`
	ActionDescription string `json:"action_description" binding:"required"`
	Deadline          string `json:"deadline" binding:"required"` // YYYY-MM-DD
}

// AssignViolation handles assigning a responsible person to a violation and creating a corrective action.
func (vc *ViolationsController) AssignViolation(c *gin.Context) {
	id := c.Param("id")
	violationID, err := strconv.Atoi(id)
	if err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid violation ID", err.Error())
		return
	}

	var req assignViolationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid payload: assigned_to, action_description, and deadline are required", err.Error())
		return
	}

	userIDVal, _ := c.Get(middleware.CtxUserID)
	userID := userIDVal.(int)

	// 1. Fetch current violation record
	var currentCode, currentDesc string
	var mineID int
	err = database.DB.QueryRow(`SELECT violation_code, description, mine_id FROM violations WHERE id = ?`, violationID).Scan(&currentCode, &currentDesc, &mineID)
	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Violation not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// 2. Verify assignee user exists
	var assigneeName string
	err = database.DB.QueryRow(`SELECT full_name FROM users WHERE id = ? AND status = 'ACTIVE'`, req.AssignedTo).Scan(&assigneeName)
	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusBadRequest, "Assignee user does not exist or is inactive", "invalid assignee")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	tx, err := database.DB.Begin()
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	// 3. Create corrective_actions record (status='ASSIGNED')
	res, err := tx.Exec(`
		INSERT INTO corrective_actions (violation_id, assigned_to, action_description, deadline, status)
		VALUES (?, ?, ?, ?, 'ASSIGNED')`,
		violationID, req.AssignedTo, req.ActionDescription, req.Deadline)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to create corrective action record", err.Error())
		return
	}
	actionID, _ := res.LastInsertId()

	// 4. Update violation to IN_PROGRESS and set responsible_person + deadline
	_, err = tx.Exec(`
		UPDATE violations 
		SET responsible_person = ?, status = 'IN_PROGRESS', deadline = ?, updated_at = NOW() 
		WHERE id = ?`,
		req.AssignedTo, req.Deadline, violationID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to update violation status", err.Error())
		return
	}

	// 5. Notify the assignee
	title := "Corrective Action Assigned To You"
	message := fmt.Sprintf("You have been assigned to resolve Violation %s (Deadline: %s): %s", currentCode, req.Deadline, req.ActionDescription)
	_, _ = tx.Exec(`
		INSERT INTO notifications (recipient_id, title, message, severity, type, is_read)
		VALUES (?, ?, ?, 'INFO', 'CORRECTIVE_ACTION', FALSE)`,
		req.AssignedTo, title, message)

	if err := tx.Commit(); err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to commit assignment transaction", err.Error())
		return
	}

	// 6. Log audit action
	utils.LogAudit(userID, "VIOLATION_ASSIGNED", "VIOLATIONS", currentCode,
		map[string]interface{}{
			"violation_id":         violationID,
			"assigned_to":          req.AssignedTo,
			"assignee_name":        assigneeName,
			"action_description":   req.ActionDescription,
			"deadline":             req.Deadline,
			"corrective_action_id": actionID,
		}, c.ClientIP())

	utils.Success(c, http.StatusOK, "Violation assigned and corrective action created successfully", gin.H{
		"violation_id":         violationID,
		"corrective_action_id": actionID,
		"assigned_to":          req.AssignedTo,
		"status":               "IN_PROGRESS",
	})
}

// CheckSLAs triggers an immediate manual evaluation of SLA escalation rules across violations and grievances.
func (vc *ViolationsController) CheckSLAs(c *gin.Context) {
	result := services.CheckAndEscalateSLAs()
	utils.Success(c, http.StatusOK, "SLA escalation evaluation completed successfully", result)
}

type dismissViolationRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// DismissViolation handles dismissing an open violation directly.
func (vc *ViolationsController) DismissViolation(c *gin.Context) {
	id := c.Param("id")
	violationID, err := strconv.Atoi(id)
	if err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid violation ID", err.Error())
		return
	}

	var req dismissViolationRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(strings.TrimSpace(req.Reason)) < 10 {
		utils.Fail(c, http.StatusBadRequest, "Dismissal reason must be at least 10 characters", "reason too short or missing")
		return
	}

	userIDVal, _ := c.Get(middleware.CtxUserID)
	userID := userIDVal.(int)

	// Fetch current violation record
	var currentStatus, currentCode string
	err = database.DB.QueryRow(`SELECT status, violation_code FROM violations WHERE id = ?`, violationID).Scan(&currentStatus, &currentCode)
	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Violation not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// Check if there are active or assigned corrective actions
	var caCount int
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM corrective_actions WHERE violation_id = ? AND status IN ('ASSIGNED', 'SUBMITTED', 'IN_PROGRESS')`, violationID).Scan(&caCount)

	if caCount > 0 {
		utils.Fail(c, http.StatusBadRequest, "Cannot dismiss violation: active corrective action(s) are already assigned or in progress", "active corrective action exists")
		return
	}

	// Update status to DISMISSED
	_, err = database.DB.Exec(`UPDATE violations SET status = 'DISMISSED', updated_at = NOW() WHERE id = ?`, violationID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to dismiss violation", err.Error())
		return
	}

	// Audit log
	utils.LogAudit(userID, "VIOLATION_DISMISSED", "VIOLATIONS", currentCode, map[string]interface{}{
		"violation_id": violationID,
		"reason":       req.Reason,
	}, c.ClientIP())

	utils.Success(c, http.StatusOK, "Violation dismissed successfully", gin.H{
		"id":     violationID,
		"status": "DISMISSED",
		"reason": req.Reason,
	})
}



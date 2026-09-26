package controllers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"coal-governance-backend/config"
	"coal-governance-backend/database"
	"coal-governance-backend/middleware"
	"coal-governance-backend/models"
	"coal-governance-backend/utils"
)

type CorrectiveActionsController struct {
	Cfg *config.Config
}

func NewCorrectiveActionsController(cfg *config.Config) *CorrectiveActionsController {
	return &CorrectiveActionsController{Cfg: cfg}
}

// ListCorrectiveActions fetches corrective actions, running the check-overdue logic first.
func (cac *CorrectiveActionsController) ListCorrectiveActions(c *gin.Context) {
	// First run the escalation and status check so that listings are always up to date
	runEscalationCheck()

	query := `
		SELECT ca.id, ca.violation_id, v.violation_code, v.description AS violation_desc, m.mine_name, v.severity,
		       ca.assigned_to, u1.full_name AS assigned_to_name, ca.action_description, ca.deadline,
		       ca.submitted_at, ca.verified_by, u2.full_name AS verified_by_name, ca.verified_at,
		       ca.escalation_level, ca.status, ca.evidence_photo_path, ca.resolution_gps_latitude, ca.resolution_gps_longitude, ca.resolution_notes, ca.created_at
		FROM corrective_actions ca
		JOIN violations v ON v.id = ca.violation_id
		JOIN mines m ON m.id = v.mine_id
		JOIN users u1 ON u1.id = ca.assigned_to
		LEFT JOIN users u2 ON u2.id = ca.verified_by
		WHERE 1=1`
	args := []interface{}{}

	if assignedTo := c.Query("assigned_to"); assignedTo != "" {
		query += " AND ca.assigned_to = ?"
		args = append(args, assignedTo)
	}
	if status := c.Query("status"); status != "" {
		query += " AND ca.status = ?"
		args = append(args, status)
	}
	if violationID := c.Query("violation_id"); violationID != "" {
		query += " AND ca.violation_id = ?"
		args = append(args, violationID)
	}

	query += " ORDER BY ca.created_at DESC"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to fetch corrective actions", err.Error())
		return
	}
	defer rows.Close()

	actions := []models.CorrectiveAction{}
	for rows.Next() {
		var ca models.CorrectiveAction
		var deadlineVal []uint8
		var subAt, verAt sql.NullTime
		var verBy sql.NullInt64
		var verByName sql.NullString
		var evPhoto, resNotes sql.NullString
		var resLat, resLng sql.NullFloat64

		err := rows.Scan(&ca.ID, &ca.ViolationID, &ca.ViolationCode, &ca.ViolationDesc, &ca.MineName, &ca.Severity,
			&ca.AssignedTo, &ca.AssignedToName, &ca.ActionDescription, &deadlineVal,
			&subAt, &verBy, &verByName, &verAt,
			&ca.EscalationLevel, &ca.Status,
			&evPhoto, &resLat, &resLng, &resNotes,
			&ca.CreatedAt)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to parse corrective action", err.Error())
			return
		}

		if deadlineVal != nil {
			ca.Deadline = string(deadlineVal)
		}
		if subAt.Valid {
			ca.SubmittedAt = &subAt.Time
		}
		if verAt.Valid {
			ca.VerifiedAt = &verAt.Time
		}
		if verBy.Valid {
			id := int(verBy.Int64)
			ca.VerifiedBy = &id
		}
		if verByName.Valid {
			ca.VerifiedByName = verByName.String
		}
		if evPhoto.Valid {
			ca.EvidencePhotoPath = &evPhoto.String
		}
		if resLat.Valid {
			ca.ResolutionGPSLat = &resLat.Float64
		}
		if resLng.Valid {
			ca.ResolutionGPSLng = &resLng.Float64
		}
		if resNotes.Valid {
			ca.ResolutionNotes = &resNotes.String
		}

		actions = append(actions, ca)
	}

	utils.Success(c, http.StatusOK, "Corrective actions fetched successfully", actions)
}

type correctiveActionRequest struct {
	ViolationID       int    `json:"violation_id" binding:"required"`
	AssignedTo        int    `json:"assigned_to" binding:"required"`
	ActionDescription string `json:"action_description" binding:"required"`
	Deadline          string `json:"deadline" binding:"required"` // YYYY-MM-DD
}

// CreateCorrectiveAction assigns a corrective action for an open violation.
func (cac *CorrectiveActionsController) CreateCorrectiveAction(c *gin.Context) {
	var req correctiveActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid payload", err.Error())
		return
	}

	userID, _ := c.Get(middleware.CtxUserID)

	tx, err := database.DB.Begin()
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	// 1. Create corrective action record
	res, err := tx.Exec(`
		INSERT INTO corrective_actions (violation_id, assigned_to, action_description, deadline, status)
		VALUES (?, ?, ?, ?, 'ASSIGNED')`,
		req.ViolationID, req.AssignedTo, req.ActionDescription, req.Deadline)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to create corrective action", err.Error())
		return
	}

	newID, _ := res.LastInsertId()

	// 2. Update violation status to IN_PROGRESS and link assignee
	_, err = tx.Exec(`
		UPDATE violations SET status = 'IN_PROGRESS', responsible_person = ?, deadline = ? WHERE id = ?`,
		req.AssignedTo, req.Deadline, req.ViolationID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to update violation link", err.Error())
		return
	}

	// Fetch violation code for audit log
	var violationCode string
	_ = tx.QueryRow(`SELECT violation_code FROM violations WHERE id = ?`, req.ViolationID).Scan(&violationCode)

	if err := tx.Commit(); err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to commit assignment", err.Error())
		return
	}

	utils.LogAudit(userID.(int), "CORRECTIVE_ACTION_ASSIGNED", "CORRECTIVE_ACTIONS", strconv.FormatInt(newID, 10),
		map[string]interface{}{"violation_code": violationCode, "assigned_to": req.AssignedTo}, c.ClientIP())

	utils.Success(c, http.StatusCreated, "Corrective action assigned successfully", gin.H{"id": newID})
}

// ResolveWithEvidence handles resolving a corrective action with mandatory photo evidence and GPS coordinates.
func (cac *CorrectiveActionsController) ResolveWithEvidence(c *gin.Context) {
	id := c.Param("id")
	actionID, err := strconv.Atoi(id)
	if err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid corrective action ID", err.Error())
		return
	}

	userIDVal, _ := c.Get(middleware.CtxUserID)
	userID := userIDVal.(int)
	userRoleVal, _ := c.Get(middleware.CtxRoleKey)
	userRole := ""
	if userRoleVal != nil {
		userRole = userRoleVal.(string)
	}

	// Fetch corrective action, violation, and mine details
	var assignedTo, violationID, mineID int
	var currentStatus, violationCode, mineName string
	var mineLat, mineLng sql.NullFloat64

	err = database.DB.QueryRow(`
		SELECT ca.assigned_to, ca.violation_id, ca.status, v.violation_code, v.mine_id, m.mine_name, m.latitude, m.longitude
		FROM corrective_actions ca
		JOIN violations v ON v.id = ca.violation_id
		JOIN mines m ON m.id = v.mine_id
		WHERE ca.id = ?`, actionID).Scan(&assignedTo, &violationID, &currentStatus, &violationCode, &mineID, &mineName, &mineLat, &mineLng)

	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Corrective action not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Database query failed", err.Error())
		return
	}

	// Verify authorization: Assigned worker OR Super Admin
	if assignedTo != userID && userRole != models.RoleSuperAdmin {
		utils.Fail(c, http.StatusForbidden, "You are not assigned to resolve this corrective action", "forbidden")
		return
	}

	if currentStatus == "SUBMITTED" || currentStatus == "CLOSED" || currentStatus == "VERIFIED" {
		utils.Fail(c, http.StatusBadRequest, fmt.Sprintf("Corrective action is already in '%s' status", currentStatus), "invalid state")
		return
	}

	// Parse GPS coordinates
	latStr := strings.TrimSpace(c.PostForm("latitude"))
	if latStr == "" {
		latStr = strings.TrimSpace(c.PostForm("gps_latitude"))
	}
	lngStr := strings.TrimSpace(c.PostForm("longitude"))
	if lngStr == "" {
		lngStr = strings.TrimSpace(c.PostForm("gps_longitude"))
	}
	if latStr == "" || lngStr == "" {
		utils.Fail(c, http.StatusBadRequest, "GPS coordinates (latitude and longitude) are required", "missing GPS")
		return
	}
	lat, err1 := strconv.ParseFloat(latStr, 64)
	lng, err2 := strconv.ParseFloat(lngStr, 64)
	if err1 != nil || err2 != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid GPS coordinates format", "invalid GPS")
		return
	}

	// Parse photo evidence
	file, header, fileErr := c.Request.FormFile("evidence")
	if fileErr != nil {
		utils.Fail(c, http.StatusBadRequest, "Evidence photo is required for resolution", "missing evidence photo")
		return
	}
	defer file.Close()

	resolutionNotes := strings.TrimSpace(c.PostForm("resolution_notes"))
	if resolutionNotes == "" {
		utils.Fail(c, http.StatusBadRequest, "Resolution notes are required", "missing resolution notes")
		return
	}

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		utils.Fail(c, http.StatusBadRequest, "Invalid evidence file type. Only JPG, PNG, and WebP images are accepted.", "invalid file type")
		return
	}
	uploadDir := "./uploads"
	if cac.Cfg != nil && cac.Cfg.UploadDir != "" {
		uploadDir = cac.Cfg.UploadDir
	}
	_ = os.MkdirAll(uploadDir, os.ModePerm)
	filename := fmt.Sprintf("resolution_%d_%d%s", actionID, time.Now().Unix(), ext)
	evidencePath := filepath.Join(uploadDir, filename)

	out, err := os.Create(evidencePath)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to save evidence file", err.Error())
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to write evidence file", err.Error())
		return
	}
	evidencePath = filepath.ToSlash(evidencePath)

	// Calculate distance to mine & geofence anomaly check
	var distanceM float64 = 0
	isOffSite := false
	radiusM := 500.0
	if cac.Cfg != nil && cac.Cfg.GeofenceRadiusM > 0 {
		radiusM = cac.Cfg.GeofenceRadiusM
	}

	if mineLat.Valid && mineLng.Valid {
		distanceM = HaversineDistance(lat, lng, mineLat.Float64, mineLng.Float64)
		if distanceM > radiusM {
			isOffSite = true
		}
	}

	tx, err := database.DB.Begin()
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	// If off-site resolution detected, flag an anomaly (does NOT block submission)
	if isOffSite {
		desc := fmt.Sprintf("Corrective action #%d resolved %.1fm away from %s (geofence perimeter: %.0fm)", actionID, distanceM, mineName, radiusM)
		_, _ = tx.Exec(`
			INSERT INTO anomalies (mine_id, anomaly_type, description, detected_value, expected_value, severity, status)
			VALUES (?, 'OFF_SITE_RESOLUTION', ?, ?, ?, 'MEDIUM', 'NEW')`,
			mineID, desc, distanceM, radiusM)
	}

	// Update corrective action record to SUBMITTED
	_, err = tx.Exec(`
		UPDATE corrective_actions 
		SET status = 'SUBMITTED', submitted_at = NOW(), evidence_photo_path = ?, resolution_gps_latitude = ?, resolution_gps_longitude = ?, resolution_notes = ?
		WHERE id = ?`,
		evidencePath, lat, lng, resolutionNotes, actionID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to update corrective action", err.Error())
		return
	}

	// Violation status remains IN_PROGRESS until supervisor verifies it

	// Notify Safety Officers and Mine Managers
	var managerIDs []int
	mgrRows, err := database.DB.Query(`
		SELECT u.id FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE r.role_key IN ('MINE_MANAGER', 'SAFETY_OFFICER', 'SUPER_ADMIN') AND u.status = 'ACTIVE'`)
	if err == nil {
		for mgrRows.Next() {
			var uid int
			if err := mgrRows.Scan(&uid); err == nil && uid != userID {
				managerIDs = append(managerIDs, uid)
			}
		}
		mgrRows.Close()

		title := fmt.Sprintf("Resolution Submitted: Violation %s", violationCode)
		msg := fmt.Sprintf("Assignee submitted photo evidence and GPS resolution for action #%d (Violation %s). Pending review.", actionID, violationCode)
		for _, uid := range managerIDs {
			_, _ = tx.Exec(`
				INSERT INTO notifications (recipient_id, title, message, severity, type, is_read)
				VALUES (?, ?, ?, 'INFO', 'CORRECTIVE_ACTION_SUBMITTED', FALSE)`,
				uid, title, msg)
		}
	}

	if err := tx.Commit(); err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to commit resolution transaction", err.Error())
		return
	}

	// Audit Trail Log
	utils.LogAudit(userID, "CORRECTIVE_ACTION_RESOLVED", "CORRECTIVE_ACTIONS", strconv.Itoa(actionID), map[string]interface{}{
		"violation_code": violationCode,
		"evidence_path":  evidencePath,
		"lat":            lat,
		"lng":            lng,
		"distance_m":     distanceM,
		"off_site":       isOffSite,
		"notes":          resolutionNotes,
	}, c.ClientIP())

	utils.Success(c, http.StatusOK, "Corrective action resolved with geotagged evidence and submitted for review", gin.H{
		"id":                 actionID,
		"status":             "SUBMITTED",
		"off_site_anomaly":   isOffSite,
		"distance_to_mine_m": distanceM,
		"evidence_path":      evidencePath,
	})
}

// SubmitAction handles a worker submitting a completed corrective action.
func (cac *CorrectiveActionsController) SubmitAction(c *gin.Context) {
	id := c.Param("id")

	userID, _ := c.Get(middleware.CtxUserID)

	// Fetch corrective action and check assignment
	var assignedTo, violationID int
	var currentStatus string
	err := database.DB.QueryRow(`SELECT assigned_to, violation_id, status FROM corrective_actions WHERE id = ?`, id).Scan(&assignedTo, &violationID, &currentStatus)
	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Corrective action not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Query failed", err.Error())
		return
	}

	// Verify that the submitter is the assigned worker
	if assignedTo != userID.(int) {
		utils.Fail(c, http.StatusForbidden, "You are not assigned to this corrective action", "forbidden")
		return
	}

	tx, err := database.DB.Begin()
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	// Update status to SUBMITTED
	_, err = tx.Exec(`
		UPDATE corrective_actions SET status = 'SUBMITTED', submitted_at = NOW() WHERE id = ?`, id)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to update corrective action status", err.Error())
		return
	}

	// Update violation to RESOLVED (pending verification)
	_, err = tx.Exec(`
		UPDATE violations SET status = 'RESOLVED' WHERE id = ?`, violationID)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to update violation status", err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to commit submission", err.Error())
		return
	}

	utils.LogAudit(userID.(int), "CORRECTIVE_ACTION_SUBMITTED", "CORRECTIVE_ACTIONS", id, nil, c.ClientIP())

	utils.Success(c, http.StatusOK, "Corrective action submitted for review", nil)
}

// VerifyAction handles supervisor review (Verify and Close).
func (cac *CorrectiveActionsController) VerifyAction(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Approved          *bool  `json:"approved"` // true to close, false to reject back to ASSIGNED
		Action            string `json:"action"`   // "APPROVE" or "REJECT"
		Remarks           string `json:"remarks"`
		VerificationNotes string `json:"verification_notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid payload", err.Error())
		return
	}

	isApproved := false
	if req.Approved != nil && *req.Approved {
		isApproved = true
	}
	if strings.ToUpper(req.Action) == "APPROVE" {
		isApproved = true
	}

	remarks := req.Remarks
	if remarks == "" {
		remarks = req.VerificationNotes
	}

	userID, _ := c.Get(middleware.CtxUserID)

	// Fetch corrective action details
	var violationID int
	var currentStatus string
	err := database.DB.QueryRow(`SELECT violation_id, status FROM corrective_actions WHERE id = ?`, id).Scan(&violationID, &currentStatus)
	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Corrective action not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Query failed", err.Error())
		return
	}

	if currentStatus != "SUBMITTED" {
		utils.Fail(c, http.StatusBadRequest, "Corrective action is not submitted for review", "invalid state")
		return
	}

	tx, err := database.DB.Begin()
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	if isApproved {
		// Close the corrective action with VERIFIED status
		_, err = tx.Exec(`
			UPDATE corrective_actions SET status = 'VERIFIED', verified_by = ?, verified_at = NOW() WHERE id = ?`,
			userID, id)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to verify corrective action", err.Error())
			return
		}

		// Close the violation with RESOLVED status
		_, err = tx.Exec(`UPDATE violations SET status = 'RESOLVED' WHERE id = ?`, violationID)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to update violation status", err.Error())
			return
		}

		utils.LogAudit(userID.(int), "CORRECTIVE_ACTION_VERIFIED", "CORRECTIVE_ACTIONS", id, map[string]interface{}{"remarks": remarks}, c.ClientIP())
	} else {
		// Reject it back to ASSIGNED status
		_, err = tx.Exec(`
			UPDATE corrective_actions SET status = 'ASSIGNED', submitted_at = NULL WHERE id = ?`, id)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to reject corrective action", err.Error())
			return
		}

		// Set violation back to IN_PROGRESS
		_, err = tx.Exec(`UPDATE violations SET status = 'IN_PROGRESS' WHERE id = ?`, violationID)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to reset violation status", err.Error())
			return
		}

		utils.LogAudit(userID.(int), "CORRECTIVE_ACTION_REJECTED", "CORRECTIVE_ACTIONS", id, map[string]interface{}{"remarks": remarks}, c.ClientIP())
	}

	if err := tx.Commit(); err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to commit verification", err.Error())
		return
	}

	utils.Success(c, http.StatusOK, "Verification processed successfully", nil)
}

// TriggerEscalationCheck is an endpoint to manually trigger a deadline check.
func (cac *CorrectiveActionsController) TriggerEscalationCheck(c *gin.Context) {
	runEscalationCheck()
	utils.Success(c, http.StatusOK, "Escalation check completed", nil)
}

// runEscalationCheck scans for overdue actions and increments escalation levels and raises alerts.
func runEscalationCheck() {
	// Find all ASSIGNED or OVERDUE corrective actions that have exceeded their deadline
	now := time.Now().Format("2006-01-02")
	rows, err := database.DB.Query(`
		SELECT ca.id, ca.violation_id, v.violation_code, v.severity, ca.deadline, ca.escalation_level, ca.status
		FROM corrective_actions ca
		JOIN violations v ON v.id = ca.violation_id
		WHERE (ca.status = 'ASSIGNED' OR ca.status = 'OVERDUE')
		  AND ca.deadline < ?`, now)
	if err != nil {
		fmt.Printf("Error querying overdue corrective actions: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, violationID, escalationLevel int
		var violationCode, severity, deadline, status string

		err = rows.Scan(&id, &violationID, &violationCode, &severity, &deadline, &escalationLevel, &status)
		if err != nil {
			continue
		}

		// Mark status as OVERDUE
		if status != "OVERDUE" {
			_, _ = database.DB.Exec(`UPDATE corrective_actions SET status = 'OVERDUE' WHERE id = ?`, id)
			_, _ = database.DB.Exec(`UPDATE violations SET status = 'OVERDUE' WHERE id = ?`, violationID)
			utils.LogAudit(0, "CORRECTIVE_ACTION_OVERDUE", "CORRECTIVE_ACTIONS", strconv.Itoa(id), map[string]interface{}{"violation_code": violationCode}, "127.0.0.1")
		}

		// For HIGH/CRITICAL severity, escalate based on how overdue it is.
		if severity == "HIGH" || severity == "CRITICAL" {
			dlTime, err := time.Parse("2006-01-02", deadline)
			if err == nil {
				daysOverdue := int(time.Since(dlTime).Hours() / 24)

				newEscLevel := 0
				if daysOverdue >= 5 {
					newEscLevel = 3 // Corporate Manager
				} else if daysOverdue >= 3 {
					newEscLevel = 2 // Mine Manager
				} else if daysOverdue >= 1 {
					newEscLevel = 1 // Safety Officer
				}

				if newEscLevel > escalationLevel {
					_, _ = database.DB.Exec(`UPDATE corrective_actions SET escalation_level = ? WHERE id = ?`, newEscLevel, id)
					utils.LogAudit(0, "CORRECTIVE_ACTION_ESCALATED", "CORRECTIVE_ACTIONS", strconv.Itoa(id),
						map[string]interface{}{
							"violation_code":   violationCode,
							"escalation_level": newEscLevel,
							"days_overdue":     daysOverdue,
						}, "127.0.0.1")

					// Generate automated alerts (in-app notifications)
					generateEscalationNotifications(violationID, violationCode, severity, newEscLevel)
				}
			}
		}
	}
}

// generateEscalationNotifications inserts alerts into notifications table.
func generateEscalationNotifications(violationID int, violationCode, severity string, level int) {
	// Determine target role for notification
	var roleKey string
	switch level {
	case 1:
		roleKey = "SAFETY_OFFICER"
	case 2:
		roleKey = "MINE_MANAGER"
	case 3:
		roleKey = "CORPORATE_MANAGER"
	default:
		return
	}

	// Fetch users holding this role
	rows, err := database.DB.Query(`
		SELECT u.id FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE r.role_key = ? AND u.status = 'ACTIVE'`, roleKey)
	if err != nil {
		return
	}
	defer rows.Close()

	title := fmt.Sprintf("ESCALATION LEVEL %d: Overdue %s Violation", level, severity)
	message := fmt.Sprintf("Violation %s (ID #%d) is overdue and has been escalated to Level %d. Immediate review required.", violationCode, violationID, level)

	for rows.Next() {
		var uid int
		if err := rows.Scan(&uid); err == nil {
			_, _ = database.DB.Exec(`
				INSERT INTO notifications (recipient_id, title, message, severity, type, is_read)
				VALUES (?, ?, ?, 'CRITICAL', 'ESCALATION', FALSE)`,
				uid, title, message)
		}
	}
}

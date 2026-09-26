package controllers

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"coal-governance-backend/config"
	"coal-governance-backend/database"
	"coal-governance-backend/middleware"
	"coal-governance-backend/models"
	"coal-governance-backend/utils"
)

type AttendanceController struct {
	Cfg *config.Config
}

func NewAttendanceController(cfg *config.Config) *AttendanceController {
	return &AttendanceController{Cfg: cfg}
}

// HaversineDistance computes the great-circle distance between two GPS coordinates in meters.
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0 // meters

	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0

	lat1Rad := lat1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

type selfCheckinRequest struct {
	MineID             int      `json:"mine_id" binding:"required"`
	WorkerID           int      `json:"worker_id" binding:"required"`
	Lat                float64  `json:"lat" binding:"required"`
	Lng                float64  `json:"lng" binding:"required"`
	IsMockLocation     bool     `json:"is_mock_location"`
	DeviceUptimeMs     *int64   `json:"device_uptime_ms"`
	ClientReportedTime *string  `json:"client_reported_time"`
	LivenessPassed     *bool    `json:"liveness_passed"`
}

// SelfCheckin handles mobile/web worker self-attendance verification with multi-vector anti-spoofing defense.
func (ac *AttendanceController) SelfCheckin(c *gin.Context) {
	var req selfCheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid checkin payload", err.Error())
		return
	}

	userIDVal, _ := c.Get(middleware.CtxUserID)
	userID := 0
	if userIDVal != nil {
		userID = userIDVal.(int)
	}

	// 1. Fetch mine coordinates and name
	var mineName string
	var mineLat, mineLng sql.NullFloat64
	err := database.DB.QueryRow(`SELECT mine_name, latitude, longitude FROM mines WHERE id = ?`, req.MineID).Scan(&mineName, &mineLat, &mineLng)
	if err == sql.ErrNoRows {
		utils.Fail(c, http.StatusNotFound, "Mine site not found", "not found")
		return
	} else if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Database error querying mine", err.Error())
		return
	}

	if !mineLat.Valid || !mineLng.Valid {
		utils.Fail(c, http.StatusBadRequest, "Mine site has no registered GPS anchor point", "no coords")
		return
	}

	// 2. Compute Haversine distance in meters
	distanceM := math.Round(HaversineDistance(req.Lat, req.Lng, mineLat.Float64, mineLng.Float64)*100) / 100

	// 3. Mock GPS detection check
	if req.IsMockLocation {
		// Log Critical Anomaly
		_, _ = database.DB.Exec(`
			INSERT INTO anomalies (mine_id, worker_id, anomaly_type, description, detected_value, expected_value, severity, status)
			VALUES (?, ?, 'MOCK_LOCATION', ?, 1.0, 0.0, 'CRITICAL', 'NEW')`,
			req.MineID, req.WorkerID, fmt.Sprintf("Mock GPS provider flag active during check-in for worker #%d at coords (%f, %f)", req.WorkerID, req.Lat, req.Lng))

		utils.LogAudit(userID, "ATTENDANCE_MOCK_LOCATION_REJECTED", "ATTENDANCE", strconv.Itoa(req.WorkerID),
			map[string]interface{}{"mine_id": req.MineID, "worker_id": req.WorkerID, "lat": req.Lat, "lng": req.Lng}, c.ClientIP())

		utils.Fail(c, http.StatusForbidden, "Spoofing detected: Mock GPS provider active (isFromMockProvider)", "MOCK_LOCATION")
		return
	}

	// 4. Geofence boundary check
	radiusM := 500.0
	if ac.Cfg != nil && ac.Cfg.GeofenceRadiusM > 0 {
		radiusM = ac.Cfg.GeofenceRadiusM
	}

	if distanceM > radiusM {
		// Log Geofence Breach Anomaly
		_, _ = database.DB.Exec(`
			INSERT INTO anomalies (mine_id, worker_id, anomaly_type, description, detected_value, expected_value, severity, status)
			VALUES (?, ?, 'GEOFENCE_BREACH', ?, ?, ?, 'HIGH', 'NEW')`,
			req.MineID, req.WorkerID, fmt.Sprintf("Worker #%d checked in %.1fm away from %s (perimeter limit: %.0fm)", req.WorkerID, distanceM, mineName, radiusM), distanceM, radiusM)

		utils.LogAudit(userID, "ATTENDANCE_GEOFENCE_BREACH_REJECTED", "ATTENDANCE", strconv.Itoa(req.WorkerID),
			map[string]interface{}{"mine_id": req.MineID, "worker_id": req.WorkerID, "distance_m": distanceM, "radius_m": radiusM}, c.ClientIP())

		utils.Fail(c, http.StatusForbidden, fmt.Sprintf("Outside mine perimeter: distance %.1fm exceeds %.0fm geofence radius", distanceM, radiusM), "OUTSIDE_GEOFENCE")
		return
	}

	// 5. Server-authoritative time audit & clock skew detection
	now := time.Now().UTC()
	recordDate := now.Format("2006-01-02")
	tamperFlag := false

	var clientReportedTimeVal *time.Time
	if req.ClientReportedTime != nil && *req.ClientReportedTime != "" {
		parsedTime, parseErr := time.Parse(time.RFC3339, *req.ClientReportedTime)
		if parseErr != nil {
			parsedTime, parseErr = time.Parse("2006-01-02T15:04:05", *req.ClientReportedTime)
		}
		if parseErr == nil {
			clientReportedTimeVal = &parsedTime
			diff := now.Sub(parsedTime)
			if math.Abs(diff.Minutes()) > 10.0 {
				tamperFlag = true
				_, _ = database.DB.Exec(`
					INSERT INTO anomalies (mine_id, worker_id, anomaly_type, description, detected_value, expected_value, severity, status)
					VALUES (?, ?, 'TIME_ANOMALY', ?, ?, 0.0, 'MEDIUM', 'NEW')`,
					req.MineID, req.WorkerID, fmt.Sprintf("Clock skew anomaly: Client time (%s) differs by %.1f mins from server time (%s)", *req.ClientReportedTime, diff.Minutes(), now.Format(time.RFC3339)), math.Abs(diff.Minutes()))
			}
		}
	}

	// 6. Tier 2 Liveness Gesture Check
	livenessPassed := true
	if req.LivenessPassed != nil {
		livenessPassed = *req.LivenessPassed
		if !livenessPassed {
			tamperFlag = true
			_, _ = database.DB.Exec(`
				INSERT INTO anomalies (mine_id, worker_id, anomaly_type, description, detected_value, expected_value, severity, status)
				VALUES (?, ?, 'LIVENESS_FAILED', ?, 0.0, 1.0, 'LOW', 'NEW')`,
				req.MineID, req.WorkerID, fmt.Sprintf("Liveness gesture test failed or skipped during check-in for worker #%d", req.WorkerID))
		}
	}

	// 7. Insert into append-only attendance_checkin_events table
	eventRes, err := database.DB.Exec(`
		INSERT INTO attendance_checkin_events (
			mine_id, worker_id, lat, lng, distance_from_mine_m, event_type,
			is_mock_location, device_uptime_ms, client_reported_time, tamper_flag, liveness_passed, recorded_at
		) VALUES (?, ?, ?, ?, ?, 'CHECKIN', ?, ?, ?, ?, ?, ?)`,
		req.MineID, req.WorkerID, req.Lat, req.Lng, distanceM,
		req.IsMockLocation, req.DeviceUptimeMs, clientReportedTimeVal, tamperFlag, livenessPassed, now)

	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to record checkin event", err.Error())
		return
	}
	eventID, _ := eventRes.LastInsertId()

	// 8. Upsert daily attendance summary record
	var markedByVal interface{} = nil
	if userID > 0 {
		markedByVal = userID
	}

	_, _ = database.DB.Exec(`
		INSERT INTO attendance (
			mine_id, worker_id, record_date, status, shift, overtime_hours,
			checkin_lat, checkin_lng, distance_from_mine_m, is_mock_location,
			device_uptime_ms, client_reported_time, tamper_flag, marked_by
		) VALUES (?, ?, ?, 'PRESENT', 'GENERAL', 0.0, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status = 'PRESENT',
			checkin_lat = VALUES(checkin_lat),
			checkin_lng = VALUES(checkin_lng),
			distance_from_mine_m = VALUES(distance_from_mine_m),
			is_mock_location = VALUES(is_mock_location),
			device_uptime_ms = VALUES(device_uptime_ms),
			client_reported_time = VALUES(client_reported_time),
			tamper_flag = VALUES(tamper_flag) OR tamper_flag,
			marked_by = VALUES(marked_by)`,
		req.MineID, req.WorkerID, recordDate,
		req.Lat, req.Lng, distanceM, req.IsMockLocation,
		req.DeviceUptimeMs, clientReportedTimeVal, tamperFlag, markedByVal)

	// 9. Velocity-Anomaly Check (Query worker's previous checkin within 60 mins from attendance_checkin_events)
	var prevLat, prevLng float64
	var prevRecordedAt time.Time
	prevErr := database.DB.QueryRow(`
		SELECT lat, lng, recorded_at 
		FROM attendance_checkin_events 
		WHERE worker_id = ? AND id != ? AND recorded_at >= DATE_SUB(?, INTERVAL 60 MINUTE)
		ORDER BY recorded_at DESC LIMIT 1`,
		req.WorkerID, eventID, now).Scan(&prevLat, &prevLng, &prevRecordedAt)

	if prevErr == nil {
		prevDistM := HaversineDistance(req.Lat, req.Lng, prevLat, prevLng)
		elapsedHours := now.Sub(prevRecordedAt).Hours()
		if elapsedHours > 0 {
			speedKmh := (prevDistM / 1000.0) / elapsedHours
			if speedKmh > 80.0 {
				_, _ = database.DB.Exec(`
					INSERT INTO anomalies (mine_id, worker_id, anomaly_type, description, detected_value, expected_value, severity, status)
					VALUES (?, ?, 'VELOCITY_ANOMALY', ?, ?, 80.0, 'CRITICAL', 'NEW')`,
					req.MineID, req.WorkerID, fmt.Sprintf("Velocity anomaly: Worker #%d travelled %.2f km in %.1f mins (implied speed: %.1f km/h > 80 km/h limit)", req.WorkerID, prevDistM/1000.0, elapsedHours*60, speedKmh), speedKmh)
			}
		}
	}

	// 10. Scripted Batch / Clustering Check (Check if >= 5 checkins happened within 5 seconds at same location)
	var clusterCount int
	_ = database.DB.QueryRow(`
		SELECT COUNT(DISTINCT worker_id)
		FROM attendance_checkin_events
		WHERE mine_id = ? 
		  AND recorded_at >= DATE_SUB(?, INTERVAL 5 SECOND)
		  AND (lat BETWEEN ? - 0.0001 AND ? + 0.0001)
		  AND (lng BETWEEN ? - 0.0001 AND ? + 0.0001)`,
		req.MineID, now, req.Lat, req.Lat, req.Lng, req.Lng).Scan(&clusterCount)

	if clusterCount >= 5 {
		_, _ = database.DB.Exec(`
			INSERT INTO anomalies (mine_id, worker_id, anomaly_type, description, detected_value, expected_value, severity, status)
			VALUES (?, ?, 'SCRIPTED_BATCH', ?, ?, 4.0, 'HIGH', 'NEW')`,
			req.MineID, req.WorkerID, fmt.Sprintf("Contractor fraud / scripted batch cluster: %d check-ins within 5 seconds at identical coordinates", clusterCount), float64(clusterCount))
	}

	// 11. Audit Log & Response
	utils.LogAudit(userID, "ATTENDANCE_SELF_CHECKIN", "ATTENDANCE", strconv.FormatInt(eventID, 10),
		map[string]interface{}{
			"mine_id":              req.MineID,
			"worker_id":            req.WorkerID,
			"distance_from_mine_m": distanceM,
			"tamper_flag":          tamperFlag,
			"liveness_passed":      livenessPassed,
		}, c.ClientIP())

	utils.Success(c, http.StatusCreated, "Self check-in recorded successfully", gin.H{
		"event_id":             eventID,
		"mine_id":              req.MineID,
		"worker_id":            req.WorkerID,
		"distance_from_mine_m": distanceM,
		"geofence_radius_m":    radiusM,
		"tamper_flag":          tamperFlag,
		"liveness_passed":      livenessPassed,
		"status":               "PRESENT",
		"recorded_at":          now.Format(time.RFC3339),
	})
}

type markAttendanceRequest struct {
	MineID        int      `json:"mine_id" binding:"required"`
	WorkerID      *int     `json:"worker_id"`
	RecordDate    string   `json:"record_date"` // YYYY-MM-DD
	Status        string   `json:"status"`      // PRESENT, ABSENT, LEAVE, HALF_DAY
	Shift         string   `json:"shift"`       // GENERAL, SHIFT_1, SHIFT_2, SHIFT_3
	OvertimeHours float64  `json:"overtime_hours"`
	PresentCount  *int     `json:"present_count"`
	TotalCount    *int     `json:"total_count"`
	WorkerIDs     []int    `json:"worker_ids"`  // for batch marking
}

// MarkAttendance handles individual or batch worker attendance marking.
func (ac *AttendanceController) MarkAttendance(c *gin.Context) {
	var req markAttendanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	userIDVal, _ := c.Get(middleware.CtxUserID)
	userID := userIDVal.(int)

	if req.RecordDate == "" {
		req.RecordDate = time.Now().Format("2006-01-02")
	}
	if req.Status == "" {
		req.Status = "PRESENT"
	}
	if req.Shift == "" {
		req.Shift = "GENERAL"
	}

	// 1. Batch marking for multiple worker IDs
	if len(req.WorkerIDs) > 0 {
		tx, err := database.DB.Begin()
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to begin transaction", err.Error())
			return
		}
		defer tx.Rollback()

		markedCount := 0
		for _, wID := range req.WorkerIDs {
			_, err = tx.Exec(`
				INSERT INTO attendance (mine_id, worker_id, record_date, status, shift, overtime_hours, marked_by)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE status = VALUES(status), shift = VALUES(shift), overtime_hours = VALUES(overtime_hours), marked_by = VALUES(marked_by)`,
				req.MineID, wID, req.RecordDate, req.Status, req.Shift, req.OvertimeHours, userID)
			if err != nil {
				utils.Fail(c, http.StatusInternalServerError, "Failed to mark batch attendance", err.Error())
				return
			}
			markedCount++
		}

		if err := tx.Commit(); err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to commit batch attendance", err.Error())
			return
		}

		utils.LogAudit(userID, "ATTENDANCE_BATCH_MARKED", "ATTENDANCE", strconv.Itoa(req.MineID),
			map[string]interface{}{"mine_id": req.MineID, "date": req.RecordDate, "count": markedCount, "status": req.Status}, c.ClientIP())

		utils.Success(c, http.StatusOK, "Batch attendance marked successfully", gin.H{"count": markedCount})
		return
	}

	// 2. Individual worker or aggregate mine total
	res, err := database.DB.Exec(`
		INSERT INTO attendance (mine_id, worker_id, record_date, status, shift, overtime_hours, present_count, total_count, marked_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.MineID, req.WorkerID, req.RecordDate, req.Status, req.Shift, req.OvertimeHours, req.PresentCount, req.TotalCount, userID)

	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to mark attendance", err.Error())
		return
	}

	newID, _ := res.LastInsertId()
	utils.LogAudit(userID, "ATTENDANCE_MARKED", "ATTENDANCE", strconv.FormatInt(newID, 10),
		map[string]interface{}{"mine_id": req.MineID, "worker_id": req.WorkerID, "date": req.RecordDate, "status": req.Status}, c.ClientIP())

	utils.Success(c, http.StatusCreated, "Attendance recorded successfully", gin.H{"id": newID})
}

// ListAttendance lists attendance records with flexible filtering.
func (ac *AttendanceController) ListAttendance(c *gin.Context) {
	query := `
		SELECT a.id, a.mine_id, m.mine_name, a.worker_id, 
		       COALESCE(w.worker_code, ''), COALESCE(w.full_name, ''), COALESCE(w.designation, ''),
		       w.contractor_id, COALESCE(ct.company_name, ''),
		       a.record_date, a.status, a.shift, a.overtime_hours,
		       a.present_count, a.total_count, a.marked_by, COALESCE(u.full_name, ''),
		       a.checkin_lat, a.checkin_lng, a.distance_from_mine_m, COALESCE(a.is_mock_location, FALSE),
		       a.device_uptime_ms, a.client_reported_time, COALESCE(a.tamper_flag, FALSE), a.created_at
		FROM attendance a
		JOIN mines m ON m.id = a.mine_id
		LEFT JOIN workers w ON w.id = a.worker_id
		LEFT JOIN contractors ct ON ct.id = w.contractor_id
		LEFT JOIN users u ON u.id = a.marked_by
		WHERE 1=1`
	args := []interface{}{}

	if mineID := c.Query("mine_id"); mineID != "" {
		query += " AND a.mine_id = ?"
		args = append(args, mineID)
	}
	if workerID := c.Query("worker_id"); workerID != "" {
		query += " AND a.worker_id = ?"
		args = append(args, workerID)
	}
	if contractorID := c.Query("contractor_id"); contractorID != "" {
		query += " AND w.contractor_id = ?"
		args = append(args, contractorID)
	}
	if status := c.Query("status"); status != "" {
		query += " AND a.status = ?"
		args = append(args, status)
	}
	if date := c.Query("date"); date != "" {
		query += " AND a.record_date = ?"
		args = append(args, date)
	}
	if startDate := c.Query("start_date"); startDate != "" {
		query += " AND a.record_date >= ?"
		args = append(args, startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		query += " AND a.record_date <= ?"
		args = append(args, endDate)
	}

	query += " ORDER BY a.record_date DESC, a.id DESC LIMIT 300"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to fetch attendance records", err.Error())
		return
	}
	defer rows.Close()

	records := []models.Attendance{}
	for rows.Next() {
		var a models.Attendance
		var workerID, contractorID, markedBy sql.NullInt64
		var presentCount, totalCount sql.NullInt64
		var recordDate []uint8
		var cLat, cLng, distMine sql.NullFloat64
		var uptimeMs sql.NullInt64
		var clientRepTime sql.NullTime
		var isMock, tamperFlag bool

		err := rows.Scan(
			&a.ID, &a.MineID, &a.MineName, &workerID,
			&a.WorkerCode, &a.WorkerName, &a.Designation,
			&contractorID, &a.ContractorName,
			&recordDate, &a.Status, &a.Shift, &a.OvertimeHours,
			&presentCount, &totalCount, &markedBy, &a.MarkedByName,
			&cLat, &cLng, &distMine, &isMock,
			&uptimeMs, &clientRepTime, &tamperFlag, &a.CreatedAt,
		)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to parse attendance records", err.Error())
			return
		}

		a.RecordDate = string(recordDate)
		if workerID.Valid {
			wid := int(workerID.Int64)
			a.WorkerID = &wid
		}
		if contractorID.Valid {
			cid := int(contractorID.Int64)
			a.ContractorID = &cid
		}
		if markedBy.Valid {
			mid := int(markedBy.Int64)
			a.MarkedBy = &mid
		}
		if presentCount.Valid {
			pc := int(presentCount.Int64)
			a.PresentCount = &pc
		}
		if totalCount.Valid {
			tc := int(totalCount.Int64)
			a.TotalCount = &tc
		}
		if cLat.Valid {
			a.CheckinLat = &cLat.Float64
		}
		if cLng.Valid {
			a.CheckinLng = &cLng.Float64
		}
		if distMine.Valid {
			a.DistanceFromMineM = &distMine.Float64
		}
		a.IsMockLocation = isMock
		if uptimeMs.Valid {
			a.DeviceUptimeMs = &uptimeMs.Int64
		}
		if clientRepTime.Valid {
			a.ClientReportedTime = &clientRepTime.Time
		}
		a.TamperFlag = tamperFlag

		records = append(records, a)
	}

	utils.Success(c, http.StatusOK, "Attendance records fetched successfully", records)
}

// GetAttendanceReport provides aggregated attendance metrics and compliance KPIs.
func (ac *AttendanceController) GetAttendanceReport(c *gin.Context) {
	mineID := c.Query("mine_id")
	contractorID := c.Query("contractor_id")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	// 1. Total Workers in scope
	var totalWorkers int
	workerQuery := `SELECT COUNT(*) FROM workers WHERE status = 'ACTIVE'`
	workerArgs := []interface{}{}
	if mineID != "" {
		workerQuery += " AND mine_id = ?"
		workerArgs = append(workerArgs, mineID)
	}
	if contractorID != "" {
		workerQuery += " AND contractor_id = ?"
		workerArgs = append(workerArgs, contractorID)
	}
	_ = database.DB.QueryRow(workerQuery, workerArgs...).Scan(&totalWorkers)

	// 2. Status counts (Present, Absent, Leave, Half Day)
	statusQuery := `
		SELECT a.status, COUNT(*)
		FROM attendance a
		LEFT JOIN workers w ON w.id = a.worker_id
		WHERE a.worker_id IS NOT NULL AND a.record_date BETWEEN ? AND ?`
	statusArgs := []interface{}{startDate, endDate}
	if mineID != "" {
		statusQuery += " AND a.mine_id = ?"
		statusArgs = append(statusArgs, mineID)
	}
	if contractorID != "" {
		statusQuery += " AND w.contractor_id = ?"
		statusArgs = append(statusArgs, contractorID)
	}
	statusQuery += " GROUP BY a.status"

	rows, err := database.DB.Query(statusQuery, statusArgs...)
	statusBreakdown := map[string]int{"PRESENT": 0, "ABSENT": 0, "LEAVE": 0, "HALF_DAY": 0}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var st string
			var cnt int
			if err := rows.Scan(&st, &cnt); err == nil {
				statusBreakdown[st] = cnt
			}
		}
	}

	// 3. Overall rate
	totalLogged := statusBreakdown["PRESENT"] + statusBreakdown["ABSENT"] + statusBreakdown["LEAVE"] + statusBreakdown["HALF_DAY"]
	attendanceRate := 100.0
	if totalLogged > 0 {
		attendanceRate = (float64(statusBreakdown["PRESENT"]) + float64(statusBreakdown["HALF_DAY"])*0.5) / float64(totalLogged) * 100
	}

	// 4. Overtime Hours Sum
	var totalOvertime float64
	otQuery := `
		SELECT COALESCE(SUM(a.overtime_hours), 0.0)
		FROM attendance a
		LEFT JOIN workers w ON w.id = a.worker_id
		WHERE a.record_date BETWEEN ? AND ?`
	otArgs := []interface{}{startDate, endDate}
	if mineID != "" {
		otQuery += " AND a.mine_id = ?"
		otArgs = append(otArgs, mineID)
	}
	if contractorID != "" {
		otQuery += " AND w.contractor_id = ?"
		otArgs = append(otArgs, contractorID)
	}
	_ = database.DB.QueryRow(otQuery, otArgs...).Scan(&totalOvertime)

	utils.Success(c, http.StatusOK, "Attendance report summary", gin.H{
		"total_workers":    totalWorkers,
		"total_records":    totalLogged,
		"status_breakdown": statusBreakdown,
		"attendance_rate":  attendanceRate,
		"total_overtime":   totalOvertime,
		"start_date":       startDate,
		"end_date":         endDate,
	})
}

// ListWorkers returns active workers for dropdowns and attendance tables.
func (ac *AttendanceController) ListWorkers(c *gin.Context) {
	query := `
		SELECT w.id, w.mine_id, m.mine_name, w.worker_code, w.full_name, w.designation,
		       w.department_id, COALESCE(d.dept_name, ''),
		       w.contractor_id, COALESCE(ct.company_name, ''),
		       w.status, w.created_at
		FROM workers w
		JOIN mines m ON m.id = w.mine_id
		LEFT JOIN departments d ON d.id = w.department_id
		LEFT JOIN contractors ct ON ct.id = w.contractor_id
		WHERE 1=1`
	args := []interface{}{}

	if mineID := c.Query("mine_id"); mineID != "" {
		query += " AND w.mine_id = ?"
		args = append(args, mineID)
	}
	if contractorID := c.Query("contractor_id"); contractorID != "" {
		query += " AND w.contractor_id = ?"
		args = append(args, contractorID)
	}
	if status := c.Query("status"); status != "" {
		query += " AND w.status = ?"
		args = append(args, status)
	}
	query += " ORDER BY w.full_name ASC"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to query workers", err.Error())
		return
	}
	defer rows.Close()

	workers := []models.Worker{}
	for rows.Next() {
		var w models.Worker
		var deptID, contID sql.NullInt64
		err := rows.Scan(
			&w.ID, &w.MineID, &w.MineName, &w.WorkerCode, &w.FullName, &w.Designation,
			&deptID, &w.DepartmentName, &contID, &w.ContractorName,
			&w.Status, &w.CreatedAt,
		)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to parse worker row", err.Error())
			return
		}
		if deptID.Valid {
			val := int(deptID.Int64)
			w.DepartmentID = &val
		}
		if contID.Valid {
			val := int(contID.Int64)
			w.ContractorID = &val
		}
		workers = append(workers, w)
	}

	utils.Success(c, http.StatusOK, "Workers fetched successfully", workers)
}

// CreateWorker registers a new worker or contractor personnel.
func (ac *AttendanceController) CreateWorker(c *gin.Context) {
	var req struct {
		MineID       int    `json:"mine_id" binding:"required"`
		WorkerCode   string `json:"worker_code" binding:"required"`
		FullName     string `json:"full_name" binding:"required"`
		Designation  string `json:"designation"`
		DepartmentID *int   `json:"department_id"`
		ContractorID *int   `json:"contractor_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	userIDVal, _ := c.Get(middleware.CtxUserID)
	userID := userIDVal.(int)

	res, err := database.DB.Exec(`
		INSERT INTO workers (mine_id, worker_code, full_name, designation, department_id, contractor_id, status)
		VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE')`,
		req.MineID, req.WorkerCode, req.FullName, req.Designation, req.DepartmentID, req.ContractorID)

	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to create worker (code may already exist)", err.Error())
		return
	}

	newID, _ := res.LastInsertId()
	utils.LogAudit(userID, "WORKER_CREATED", "WORKERS", strconv.FormatInt(newID, 10),
		map[string]interface{}{"worker_code": req.WorkerCode, "full_name": req.FullName, "mine_id": req.MineID}, c.ClientIP())

	utils.Success(c, http.StatusCreated, "Worker created successfully", gin.H{"id": newID})
}

// ListCheckinEvents returns the append-only telemetry events log.
func (ac *AttendanceController) ListCheckinEvents(c *gin.Context) {
	query := `
		SELECT e.id, e.mine_id, m.mine_name, e.worker_id,
		       COALESCE(w.worker_code, ''), COALESCE(w.full_name, ''),
		       e.lat, e.lng, e.distance_from_mine_m, e.event_type,
		       e.is_mock_location, e.device_uptime_ms, e.client_reported_time,
		       e.tamper_flag, e.liveness_passed, e.recorded_at
		FROM attendance_checkin_events e
		JOIN mines m ON m.id = e.mine_id
		JOIN workers w ON w.id = e.worker_id
		WHERE 1=1`
	args := []interface{}{}

	if mineID := c.Query("mine_id"); mineID != "" {
		query += " AND e.mine_id = ?"
		args = append(args, mineID)
	}
	if workerID := c.Query("worker_id"); workerID != "" {
		query += " AND e.worker_id = ?"
		args = append(args, workerID)
	}
	query += " ORDER BY e.recorded_at DESC LIMIT 100"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to query checkin events", err.Error())
		return
	}
	defer rows.Close()

	events := []models.AttendanceCheckinEvent{}
	for rows.Next() {
		var ev models.AttendanceCheckinEvent
		var uptimeMs sql.NullInt64
		var clientRepTime sql.NullTime

		err := rows.Scan(
			&ev.ID, &ev.MineID, &ev.MineName, &ev.WorkerID,
			&ev.WorkerCode, &ev.WorkerName,
			&ev.Lat, &ev.Lng, &ev.DistanceFromMineM, &ev.EventType,
			&ev.IsMockLocation, &uptimeMs, &clientRepTime,
			&ev.TamperFlag, &ev.LivenessPassed, &ev.RecordedAt,
		)
		if err == nil {
			if uptimeMs.Valid {
				ev.DeviceUptimeMs = &uptimeMs.Int64
			}
			if clientRepTime.Valid {
				ev.ClientReportedTime = &clientRepTime.Time
			}
			events = append(events, ev)
		}
	}

	utils.Success(c, http.StatusOK, "Checkin events fetched", events)
}


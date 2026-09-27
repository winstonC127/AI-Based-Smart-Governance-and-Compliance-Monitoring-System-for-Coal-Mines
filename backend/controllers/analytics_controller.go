package controllers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"coal-governance-backend/config"
	"coal-governance-backend/database"
	"coal-governance-backend/utils"
)

type AnalyticsController struct {
	Cfg *config.Config
}

func NewAnalyticsController(cfg *config.Config) *AnalyticsController {
	return &AnalyticsController{Cfg: cfg}
}

// GetChartsData returns aggregated records for the 7 Chart.js dashboard charts.
func (ac *AnalyticsController) GetChartsData(c *gin.Context) {
	// Refresh risk scores asynchronously in background so chart queries return instantly (<50ms)
	go func() {
		_ = ac.RecalculateRiskForMines()
	}()

	// 1. Violations by Category
	vioCatRows, err := database.DB.Query(`
		SELECT cc.name, COUNT(*)
		FROM violations v
		JOIN compliance_categories cc ON cc.id = v.category_id
		GROUP BY cc.name`)
	type labelCount struct {
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	vioCategories := []labelCount{}
	if err == nil {
		defer vioCatRows.Close()
		for vioCatRows.Next() {
			var lc labelCount
			if err := vioCatRows.Scan(&lc.Label, &lc.Count); err == nil {
				vioCategories = append(vioCategories, lc)
			}
		}
	}

	// 2. Corrective Action Statuses
	caStatusRows, err := database.DB.Query(`SELECT status, COUNT(*) FROM corrective_actions GROUP BY status`)
	caStatuses := []labelCount{}
	if err == nil {
		defer caStatusRows.Close()
		for caStatusRows.Next() {
			var lc labelCount
			if err := caStatusRows.Scan(&lc.Label, &lc.Count); err == nil {
				caStatuses = append(caStatuses, lc)
			}
		}
	}

	// 3. Risk Distribution
	riskDistRows, err := database.DB.Query(`
		SELECT r.classification, COUNT(*)
		FROM risk_scores r
		WHERE r.computed_at = (SELECT MAX(computed_at) FROM risk_scores WHERE mine_id = r.mine_id)
		GROUP BY r.classification`)
	riskDist := []labelCount{}
	if err == nil {
		defer riskDistRows.Close()
		for riskDistRows.Next() {
			var lc labelCount
			if err := riskDistRows.Scan(&lc.Label, &lc.Count); err == nil {
				riskDist = append(riskDist, lc)
			}
		}
	}

	// 4. Mine Risk Rankings
	mineRankRows, err := database.DB.Query(`
		SELECT m.mine_name, IFNULL(r.score, 0)
		FROM mines m
		LEFT JOIN risk_scores r ON r.mine_id = m.id AND r.computed_at = (SELECT MAX(computed_at) FROM risk_scores WHERE mine_id = m.id)
		ORDER BY r.score DESC, m.mine_name ASC
		LIMIT 10`)
	type mineRank struct {
		MineName string  `json:"mine_name"`
		Score    float64 `json:"score"`
	}
	mineRanks := []mineRank{}
	if err == nil {
		defer mineRankRows.Close()
		for mineRankRows.Next() {
			var mr mineRank
			if err := mineRankRows.Scan(&mr.MineName, &mr.Score); err == nil {
				mineRanks = append(mineRanks, mr)
			}
		}
	}

	// 5. Compliance Trend (dynamic past 6 months of inspections)
	complianceTrendRows, err := database.DB.Query(`
		SELECT DATE_FORMAT(inspection_date, '%b %Y') as month_yr,
		       SUM(CASE WHEN status='APPROVED' THEN 1 ELSE 0 END) as approved_count,
		       COUNT(*) as total_count
		FROM inspections
		GROUP BY month_yr
		ORDER BY MIN(inspection_date) ASC
		LIMIT 6`)
	type trendPoint struct {
		Month string  `json:"month"`
		Rate  float64 `json:"rate"`
	}
	complianceTrend := []trendPoint{}
	if err == nil {
		defer complianceTrendRows.Close()
		for complianceTrendRows.Next() {
			var monthYr string
			var approved, total int
			if err := complianceTrendRows.Scan(&monthYr, &approved, &total); err == nil {
				rate := 100.0
				if total > 0 {
					rate = (float64(approved) / float64(total)) * 100.0
				}
				complianceTrend = append(complianceTrend, trendPoint{Month: monthYr, Rate: rate})
			}
		}
	}

	// 6. Inspection Trend
	inspectionTrendRows, err := database.DB.Query(`
		SELECT DATE_FORMAT(inspection_date, '%b %Y') as month_yr, COUNT(*)
		FROM inspections
		GROUP BY month_yr
		ORDER BY MIN(inspection_date) ASC
		LIMIT 6`)
	inspectionsTrend := []labelCount{}
	if err == nil {
		defer inspectionTrendRows.Close()
		for inspectionTrendRows.Next() {
			var lc labelCount
			if err := inspectionTrendRows.Scan(&lc.Label, &lc.Count); err == nil {
				inspectionsTrend = append(inspectionsTrend, lc)
			}
		}
	}

	// 7. Incident Trend
	incidentRows, err := database.DB.Query(`SELECT incident_type, COUNT(*) FROM incidents GROUP BY incident_type`)
	incidentTrend := []labelCount{}
	if err == nil {
		defer incidentRows.Close()
		for incidentRows.Next() {
			var lc labelCount
			if err := incidentRows.Scan(&lc.Label, &lc.Count); err == nil {
				incidentTrend = append(incidentTrend, lc)
			}
		}
	}

	utils.Success(c, http.StatusOK, "Charts data loaded", gin.H{
		"violations_by_category":     vioCategories,
		"corrective_actions_status":  caStatuses,
		"risk_distribution":          riskDist,
		"mine_risk_ranking":          mineRanks,
		"compliance_trend":           complianceTrend,
		"inspections_trend":          inspectionsTrend,
		"incidents_trend":            incidentTrend,
	})
}

// GetRiskScores returns the computed risk scores and explanations.
func (ac *AnalyticsController) GetRiskScores(c *gin.Context) {
	// Refresh risk scores in background if needed
	go func() {
		_ = ac.RecalculateRiskForMines()
	}()

	rows, err := database.DB.Query(`
		SELECT r.id, r.mine_id, m.mine_name, r.score, r.classification, r.factors_json, r.computed_at
		FROM risk_scores r
		JOIN mines m ON m.id = r.mine_id
		WHERE r.computed_at = (SELECT MAX(computed_at) FROM risk_scores WHERE mine_id = r.mine_id)
		ORDER BY r.score DESC`)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to query risk scores", err.Error())
		return
	}
	defer rows.Close()

	type riskItem struct {
		ID             int             `json:"id"`
		MineID         int             `json:"mine_id"`
		MineName       string          `json:"mine_name"`
		Score          float64         `json:"score"`
		Classification string          `json:"classification"`
		Factors        json.RawMessage `json:"factors"`
		ComputedAt     time.Time       `json:"computed_at"`
	}

	scores := []riskItem{}
	for rows.Next() {
		var ri riskItem
		var factors sql.NullString
		err := rows.Scan(&ri.ID, &ri.MineID, &ri.MineName, &ri.Score, &ri.Classification, &factors, &ri.ComputedAt)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to parse risk scores", err.Error())
			return
		}
		if factors.Valid {
			ri.Factors = json.RawMessage(factors.String)
		} else {
			ri.Factors = json.RawMessage("{}")
		}
		scores = append(scores, ri)
	}

	utils.Success(c, http.StatusOK, "Risk scores fetched", scores)
}

// TriggerRiskRecalculate handles manual recalculation trigger.
func (ac *AnalyticsController) TriggerRiskRecalculate(c *gin.Context) {
	err := ac.RecalculateRiskForMines()
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Risk recalculation failed", err.Error())
		return
	}
	utils.Success(c, http.StatusOK, "Risk scores successfully re-evaluated", nil)
}

func (ac *AnalyticsController) HandleVoiceQuery(c *gin.Context) {
	var req struct {
		Query    string `json:"query"`
		Language string `json:"language"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Valid query text is required"})
		return
	}

	// Context Gathering: Fetch current mines and risk scores
	mineRankRows, err := database.DB.Query(`
		SELECT m.id, m.mine_name, m.mine_code, IFNULL(m.state, ''), IFNULL(m.mine_type, 'OPENCAST'), IFNULL(r.score, 0)
		FROM mines m
		LEFT JOIN risk_scores r ON r.mine_id = m.id AND r.computed_at = (SELECT MAX(computed_at) FROM risk_scores WHERE mine_id = m.id)
		WHERE m.status = 'ACTIVE'
		ORDER BY r.score DESC`)
	contextMines := []map[string]interface{}{}
	if err == nil {
		defer mineRankRows.Close()
		for mineRankRows.Next() {
			var id int
			var name, code, state, mType string
			var score float64
			if err := mineRankRows.Scan(&id, &name, &code, &state, &mType, &score); err == nil {
				contextMines = append(contextMines, map[string]interface{}{
					"mine_id":    id,
					"mine_name":  name,
					"mine_code":  code,
					"state":      state,
					"mine_type":  mType,
					"risk_score": score,
				})
			}
		}
	}

	// Fetch pending violations counts
	vioRows, err := database.DB.Query(`
		SELECT m.mine_name, COUNT(v.id) 
		FROM violations v
		JOIN mines m ON v.mine_id = m.id
		WHERE v.status = 'OPEN'
		GROUP BY m.mine_name`)
	contextViolations := []map[string]interface{}{}
	totalOpenVios := 0
	if err == nil {
		defer vioRows.Close()
		for vioRows.Next() {
			var name string
			var count int
			if err := vioRows.Scan(&name, &count); err == nil {
				contextViolations = append(contextViolations, map[string]interface{}{"mine_name": name, "open_violations": count})
				totalOpenVios += count
			}
		}
	}

	// Fetch critical violations count
	var critViosCount int
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM violations WHERE status = 'OPEN' AND severity = 'CRITICAL'`).Scan(&critViosCount)

	// Fetch worker metrics
	var totalWorkers, presentToday int
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM workers WHERE status = 'ACTIVE'`).Scan(&totalWorkers)
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM attendance WHERE record_date = CURDATE() AND status = 'PRESENT'`).Scan(&presentToday)

	// Fetch latest production total
	var todayProd float64
	_ = database.DB.QueryRow(`SELECT IFNULL(SUM(production_tonnes), 0) FROM operational_data WHERE record_date = CURDATE()`).Scan(&todayProd)
	if todayProd == 0 {
		_ = database.DB.QueryRow(`SELECT IFNULL(SUM(production_tonnes), 0) FROM operational_data WHERE record_date = (SELECT MAX(record_date) FROM operational_data)`).Scan(&todayProd)
	}

	// Fetch active anomalies and incidents
	var activeAnomalies, activeIncidents int
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM anomalies WHERE status = 'NEW'`).Scan(&activeAnomalies)
	_ = database.DB.QueryRow(`SELECT COUNT(*) FROM incidents WHERE status IN ('OPEN', 'REPORTED', 'INVESTIGATING', 'ACTION_REQUIRED')`).Scan(&activeIncidents)

	contextData := map[string]interface{}{
		"total_mines_count":       len(contextMines),
		"mines_risk":              contextMines,
		"pending_violations":      contextViolations,
		"total_open_violations":   totalOpenVios,
		"critical_violations":     critViosCount,
		"total_active_workers":    totalWorkers,
		"workers_present_today":   presentToday,
		"today_production_tonnes": todayProd,
		"active_anomalies_count":  activeAnomalies,
		"active_incidents_count":  activeIncidents,
	}

	payload := map[string]interface{}{
		"query":        req.Query,
		"language":     req.Language,
		"context_data": contextData,
	}
	payloadBytes, _ := json.Marshal(payload)

	// Try calling Python AI service with 5-second timeout and localhost fallback
	baseURL := strings.TrimRight(ac.Cfg.AIServiceURL, "/")
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	aiURL := fmt.Sprintf("%s/ai/voice-assistant", baseURL)

	var answerText string
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(aiURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil && strings.Contains(aiURL, "localhost") {
		fallbackURL := strings.Replace(aiURL, "localhost", "127.0.0.1", 1)
		resp, err = client.Post(fallbackURL, "application/json", bytes.NewBuffer(payloadBytes))
	}

	if err == nil && resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			var aiResult struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
				Data    struct {
					Answer string `json:"answer"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &aiResult); err == nil && aiResult.Success && aiResult.Data.Answer != "" {
				answerText = aiResult.Data.Answer
			}
		}
	}

	// Fallback to high-accuracy domain synthesizer if AI service is offline, slow, or returned empty
	if answerText == "" {
		answerText = synthesizeVoiceAnswerGo(
			req.Query,
			req.Language,
			contextMines,
			contextViolations,
			totalOpenVios,
			critViosCount,
			totalWorkers,
			presentToday,
			todayProd,
			activeAnomalies,
			activeIncidents,
		)
	}

	// Log audit safely (user_id can be NULL if unauthenticated)
	var uid interface{} = nil
	if val, ok := c.Get("userID"); ok && val != nil {
		uid = val
	}
	database.DB.Exec(`
		INSERT INTO audit_logs (user_id, action, details)
		VALUES (?, ?, ?)`,
		uid, "Voice Query", fmt.Sprintf("Query: %s | Lang: %s", req.Query, req.Language))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Voice query processed",
		"data": gin.H{
			"answer": answerText,
		},
	})
}

// synthesizeVoiceAnswerGo provides instant, zero-latency statutory & operational answers directly in Go
func synthesizeVoiceAnswerGo(
	query string,
	language string,
	contextMines []map[string]interface{},
	contextViolations []map[string]interface{},
	totalOpenVios int,
	critViosCount int,
	totalWorkers int,
	presentToday int,
	todayProd float64,
	activeAnomalies int,
	activeIncidents int,
) string {
	q := strings.ToLower(strings.TrimSpace(query))
	lang := strings.ToLower(strings.TrimSpace(language))

	// 1. High Risk / Dangerous Mines
	if strings.Contains(q, "highest risk") || strings.Contains(q, "most dangerous") ||
		strings.Contains(q, "high risk") || strings.Contains(q, "top risk") ||
		strings.Contains(q, "risk score") || strings.Contains(q, "danger") ||
		strings.Contains(q, "jokhim") || strings.Contains(q, "khatarnak") ||
		strings.Contains(q, "जोखिम") || strings.Contains(q, "खतरनाक") ||
		strings.Contains(q, "aabathu") || strings.Contains(q, "ஆபத்து") ||
		strings.Contains(q, "pramaadam") || strings.Contains(q, "రిస్క్") {
		if len(contextMines) > 0 {
			topMine := contextMines[0]
			name, _ := topMine["mine_name"].(string)
			state, _ := topMine["state"].(string)
			mType, _ := topMine["mine_type"].(string)
			score, _ := topMine["risk_score"].(float64)

			if strings.HasPrefix(lang, "hi") {
				return fmt.Sprintf("वर्तमान में सबसे अधिक परिचालन जोखिम वाली खदान %s (%s, %s) है, जिसका जोखिम स्कोर %.1f है।", name, state, mType, score)
			} else if strings.HasPrefix(lang, "ta") {
				return fmt.Sprintf("அதிக செயல்பாட்டு ஆபத்து உள்ள சுரங்கம் %s (%s) ஆகும், இதன் ஆபத்து குறியீடு %.1f ஆகும்.", name, state, score)
			} else if strings.HasPrefix(lang, "te") {
				return fmt.Sprintf("అత్యధిక కార్యాచరణ రిస్క్ ఉన్న గని %s (%s), దీని రిస్క్ స్కోరు %.1f.", name, state, score)
			}
			return fmt.Sprintf("The mine with the highest operational risk is %s in %s (%s) with a composite risk score of %.1f.", name, state, mType, score)
		}
		return "All active mines are currently within safe baseline risk parameters under DGMS monitoring."
	}

	// 2. Violations & Statutory Compliance
	if strings.Contains(q, "violation") || strings.Contains(q, "non-compliance") ||
		strings.Contains(q, "critical") || strings.Contains(q, "breach") ||
		strings.Contains(q, "sla") || strings.Contains(q, "उल्लंघन") ||
		strings.Contains(q, "மீறல்") || strings.Contains(q, "ఉల్లంఘన") {
		if strings.HasPrefix(lang, "hi") {
			return fmt.Sprintf("वर्तमान में सभी खदानों में कुल %d वैधानिक उल्लंघन खुले हैं, जिनमें %d अति-गंभीर (Critical) उल्लंघन शामिल हैं।", totalOpenVios, critViosCount)
		} else if strings.HasPrefix(lang, "ta") {
			return fmt.Sprintf("தற்போது %d பாதுகாப்பு மீறல்கள் நிலுவையில் உள்ளன, இதில் %d தீவிர மீறல்கள் அடங்கும்.", totalOpenVios, critViosCount)
		} else if strings.HasPrefix(lang, "te") {
			return fmt.Sprintf("ప్రస్తుతం %d భద్రతా ఉల్లంఘనలు పెండింగ్‌లో ఉన్నాయి, ఇందులో %d అత్యంత తీవ్రమైనవి.", totalOpenVios, critViosCount)
		}
		return fmt.Sprintf("There are currently %d open statutory compliance violations across all mines, including %d critical severity breaches requiring immediate containment.", totalOpenVios, critViosCount)
	}

	// 3. Workers & Attendance
	if strings.Contains(q, "worker") || strings.Contains(q, "attendance") ||
		strings.Contains(q, "manpower") || strings.Contains(q, "staff") ||
		strings.Contains(q, "present") || strings.Contains(q, "mazdoor") ||
		strings.Contains(q, "shramik") || strings.Contains(q, "upasthit") ||
		strings.Contains(q, "haziri") || strings.Contains(q, "मजदूर") ||
		strings.Contains(q, "श्रमिक") || strings.Contains(q, "उपस्थिति") ||
		strings.Contains(q, "कर्मचारी") || strings.Contains(q, "தொழிலாளர்") ||
		strings.Contains(q, "வருகை") || strings.Contains(q, "కార్మికులు") ||
		strings.Contains(q, "హాజరు") {
		attPct := 0.0
		if totalWorkers > 0 {
			attPct = float64(presentToday) * 100.0 / float64(totalWorkers)
		}
		if strings.HasPrefix(lang, "hi") {
			return fmt.Sprintf("आज कुल %d पंजीकृत श्रमिकों में से %d श्रमिक उपस्थित हैं (उपस्थिति दर %.1f%%)।", totalWorkers, presentToday, attPct)
		} else if strings.HasPrefix(lang, "ta") {
			return fmt.Sprintf("இன்று பதிவுசெய்யப்பட்ட %d தொழிலாளர்களில் %d பேர் வருகை தந்துள்ளனர் (வருகை விகிதம் %.1f%%).", totalWorkers, presentToday, attPct)
		} else if strings.HasPrefix(lang, "te") {
			return fmt.Sprintf("ఈరోజు నమోదైన %d కార్మికులలో %d మంది హాజరయ్యారు (హాజరు శాతం %.1f%%).", totalWorkers, presentToday, attPct)
		}
		return fmt.Sprintf("Today's biometric workforce attendance is %d workers present out of %d active registered personnel (%.1f%% attendance).", presentToday, totalWorkers, attPct)
	}

	// 4. Production & Output
	if strings.Contains(q, "production") || strings.Contains(q, "tonnage") ||
		strings.Contains(q, "output") || strings.Contains(q, "mined") ||
		strings.Contains(q, "उत्पादन") || strings.Contains(q, "உற்பத்தி") ||
		strings.Contains(q, "ఉత్పత్తి") {
		if strings.HasPrefix(lang, "hi") {
			return fmt.Sprintf("आज का कुल कोयला उत्पादन %.2f मीट्रिक टन दर्ज किया गया है।", todayProd)
		} else if strings.HasPrefix(lang, "ta") {
			return fmt.Sprintf("இன்றைய மொத்த நிலக்கரி உற்பத்தி %.2f மெட்ரிக் டன்களாக பதிவாகியுள்ளது.", todayProd)
		} else if strings.HasPrefix(lang, "te") {
			return fmt.Sprintf("ఈరోజు మొత్తం బొగ్గు ఉత్పత్తి %.2f మెట్రిక్ టన్నులుగా నమోదైంది.", todayProd)
		}
		return fmt.Sprintf("Today's total coal extraction across monitored sites is %.2f metric tonnes.", todayProd)
	}

	// 5. Anomalies & Incidents
	if strings.Contains(q, "anomaly") || strings.Contains(q, "incident") ||
		strings.Contains(q, "emergency") || strings.Contains(q, "alarm") ||
		strings.Contains(q, "घटना") || strings.Contains(q, "விபத்து") ||
		strings.Contains(q, "ప్రమాదం") {
		if strings.HasPrefix(lang, "hi") {
			return fmt.Sprintf("सिस्टम में वर्तमान में %d सक्रिय विसंगतियां और %d खुली घटनाएं सक्रिय निगरानी में हैं।", activeAnomalies, activeIncidents)
		}
		return fmt.Sprintf("CoalGuard is tracking %d active operational anomalies and %d open incident reports under statutory investigation.", activeAnomalies, activeIncidents)
	}

	// 6. Statutory DGMS / CMR 2017 Regulations & Gas limits
	if strings.Contains(q, "methane") || strings.Contains(q, "gas") ||
		strings.Contains(q, "ch4") || strings.Contains(q, "co ") ||
		strings.Contains(q, "ventilation") || strings.Contains(q, "dgms") ||
		strings.Contains(q, "cmr") || strings.Contains(q, "rule") ||
		strings.Contains(q, "regulation") || strings.Contains(q, "statutory") {
		return "Under Coal Mines Regulations 2017 (Regulation 169), methane levels must not exceed 0.75% in general body of air and 1.25% in return airway. Carbon monoxide must remain below 50 PPM at all times."
	}

	// 7. Specific Mine lookup
	for _, m := range contextMines {
		mName, _ := m["mine_name"].(string)
		mCode, _ := m["mine_code"].(string)
		if (mName != "" && strings.Contains(q, strings.ToLower(mName))) ||
			(mCode != "" && strings.Contains(q, strings.ToLower(mCode))) {
			mScore, _ := m["risk_score"].(float64)
			mState, _ := m["state"].(string)
			mType, _ := m["mine_type"].(string)
			return fmt.Sprintf("%s (%s, %s) in %s has an operational risk score of %.1f under real-time telemetry monitoring.", mName, mCode, mType, mState, mScore)
		}
	}

	// 8. Default System Overview
	totalMines := len(contextMines)
	if strings.HasPrefix(lang, "hi") {
		return fmt.Sprintf("कोल गवर्नेंस प्लेटफॉर्म वास्तविक समय में %d खदानों, %d श्रमिकों और %d खुले उल्लंघनों की निगरानी कर रहा है।", totalMines, totalWorkers, totalOpenVios)
	} else if strings.HasPrefix(lang, "ta") {
		return fmt.Sprintf("நிலக்கரி ஆளுகை தளம் %d சுரங்கங்கள், %d தொழிலாளர்கள் மற்றும் %d பாதுகாப்பு மீறல்களை தீவிரமாக கண்காணிக்கிறது.", totalMines, totalWorkers, totalOpenVios)
	} else if strings.HasPrefix(lang, "te") {
		return fmt.Sprintf("బొగ్గు పాలన వేదిక %d గనులు, %d కార్మికులు మరియు %d భద్రతా ఉల్లంఘనలను పర్యవేక్షిస్తుంది.", totalMines, totalWorkers, totalOpenVios)
	}
	return fmt.Sprintf("The Coal Governance Platform is actively monitoring %d mines with %d registered workers and %d open safety violations.", totalMines, totalWorkers, totalOpenVios)
}

// TranslateText forwards text to Python AI service for translation with graceful fallback.
func (ac *AnalyticsController) TranslateText(c *gin.Context) {
	var req struct {
		Text           string `json:"text" binding:"required"`
		TargetLanguage string `json:"target_language"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Text is required"})
		return
	}

	if req.TargetLanguage == "" {
		req.TargetLanguage = "English"
	}

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"text":            req.Text,
		"target_language": req.TargetLanguage,
	})

	baseURL := strings.TrimRight(ac.Cfg.AIServiceURL, "/")
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	aiURL := fmt.Sprintf("%s/ai/translate", baseURL)

	var translatedText string
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(aiURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil && strings.Contains(aiURL, "localhost") {
		fallbackURL := strings.Replace(aiURL, "localhost", "127.0.0.1", 1)
		resp, err = client.Post(fallbackURL, "application/json", bytes.NewBuffer(payloadBytes))
	}

	if err == nil && resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			var aiResult struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
				Data    struct {
					TranslatedText string `json:"translated_text"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &aiResult); err == nil && aiResult.Success && aiResult.Data.TranslatedText != "" {
				translatedText = aiResult.Data.TranslatedText
			}
		}
	}

	if translatedText == "" {
		translatedText = req.Text
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Text processed",
		"data": gin.H{
			"translated_text": translatedText,
		},
	})
}

// GetAnomalies returns all registered operational, environmental, and attendance tamper anomalies.
func (ac *AnalyticsController) GetAnomalies(c *gin.Context) {
	rows, err := database.DB.Query(`
		SELECT a.id, a.mine_id, m.mine_name, a.worker_id, COALESCE(w.full_name, ''), COALESCE(w.worker_code, ''),
		       a.anomaly_type, a.description, a.detected_value, a.expected_value, a.severity, a.status, a.detected_at
		FROM anomalies a
		JOIN mines m ON m.id = a.mine_id
		LEFT JOIN workers w ON w.id = a.worker_id
		ORDER BY a.detected_at DESC LIMIT 200`)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to query anomalies", err.Error())
		return
	}
	defer rows.Close()

	type anomalyItem struct {
		ID            int       `json:"id"`
		MineID        int       `json:"mine_id"`
		MineName      string    `json:"mine_name"`
		WorkerID      *int      `json:"worker_id,omitempty"`
		WorkerName    string    `json:"worker_name,omitempty"`
		WorkerCode    string    `json:"worker_code,omitempty"`
		AnomalyType   string    `json:"anomaly_type"`
		Description   string    `json:"description"`
		DetectedValue *float64  `json:"detected_value,omitempty"`
		ExpectedValue *float64  `json:"expected_value,omitempty"`
		Severity      string    `json:"severity"`
		Status        string    `json:"status"`
		DetectedAt    time.Time `json:"detected_at"`
	}

	list := []anomalyItem{}
	for rows.Next() {
		var ai anomalyItem
		var wid sql.NullInt64
		var dVal, eVal sql.NullFloat64

		err := rows.Scan(
			&ai.ID, &ai.MineID, &ai.MineName, &wid, &ai.WorkerName, &ai.WorkerCode,
			&ai.AnomalyType, &ai.Description, &dVal, &eVal, &ai.Severity, &ai.Status, &ai.DetectedAt,
		)
		if err != nil {
			utils.Fail(c, http.StatusInternalServerError, "Failed to parse anomaly", err.Error())
			return
		}

		if wid.Valid {
			val := int(wid.Int64)
			ai.WorkerID = &val
		}
		if dVal.Valid {
			ai.DetectedValue = &dVal.Float64
		}
		if eVal.Valid {
			ai.ExpectedValue = &eVal.Float64
		}

		list = append(list, ai)
	}

	utils.Success(c, http.StatusOK, "Anomalies fetched", list)
}

// RecalculateRiskForMines runs telemetry gathering and posts stats to Python AI service.
func (ac *AnalyticsController) RecalculateRiskForMines() error {
	minesRows, err := database.DB.Query(`SELECT id, mine_name FROM mines WHERE status='ACTIVE'`)
	if err != nil {
		return err
	}
	defer minesRows.Close()

	type mineInfo struct {
		ID   int
		Name string
	}
	mines := []mineInfo{}
	for minesRows.Next() {
		var mi mineInfo
		if err := minesRows.Scan(&mi.ID, &mi.Name); err == nil {
			mines = append(mines, mi)
		}
	}

	for _, mine := range mines {
		// Gather violations counts
		var critVal, highVal, medVal, lowVal int
		_ = database.DB.QueryRow(`SELECT COUNT(*) FROM violations WHERE mine_id=? AND status IN ('OPEN','IN_PROGRESS','OVERDUE') AND severity='CRITICAL'`, mine.ID).Scan(&critVal)
		_ = database.DB.QueryRow(`SELECT COUNT(*) FROM violations WHERE mine_id=? AND status IN ('OPEN','IN_PROGRESS','OVERDUE') AND severity='HIGH'`, mine.ID).Scan(&highVal)
		_ = database.DB.QueryRow(`SELECT COUNT(*) FROM violations WHERE mine_id=? AND status IN ('OPEN','IN_PROGRESS','OVERDUE') AND severity='MEDIUM'`, mine.ID).Scan(&medVal)
		_ = database.DB.QueryRow(`SELECT COUNT(*) FROM violations WHERE mine_id=? AND status IN ('OPEN','IN_PROGRESS','OVERDUE') AND severity='LOW'`, mine.ID).Scan(&lowVal)

		// Overdue actions
		var overdueActions int
		_ = database.DB.QueryRow(`
			SELECT COUNT(*) FROM corrective_actions ca 
			JOIN violations v ON v.id = ca.violation_id 
			WHERE v.mine_id = ? AND ca.status = 'OVERDUE'`, mine.ID).Scan(&overdueActions)

		// Operational Anomaly Check
		var hasProdAnomaly, hasAttAnomaly bool
		_ = database.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM anomalies WHERE mine_id=? AND anomaly_type='PRODUCTION' AND status='NEW')`, mine.ID).Scan(&hasProdAnomaly)
		_ = database.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM anomalies WHERE mine_id=? AND anomaly_type='ATTENDANCE' AND status='NEW')`, mine.ID).Scan(&hasAttAnomaly)

		// Environmental alerts count (past 7 days warning threshold)
		var envAlerts int
		_ = database.DB.QueryRow(`
			SELECT COUNT(*) FROM environmental_data 
			WHERE mine_id = ? 
			  AND record_date >= DATE_SUB(NOW(), INTERVAL 7 DAY) 
			  AND (aqi > 150 OR water_quality_index < 65 OR dust_level > 200)`, mine.ID).Scan(&envAlerts)

		// Recurring violation in same category (>= 3 violations in past 30 days)
		var recurringCount int
		var recurringCat string
		_ = database.DB.QueryRow(`
			SELECT cc.name, COUNT(*) as cnt 
			FROM violations v 
			JOIN compliance_categories cc ON cc.id = v.category_id 
			WHERE v.mine_id = ? AND v.created_at >= DATE_SUB(NOW(), INTERVAL 30 DAY) 
			GROUP BY cc.id 
			HAVING cnt >= 3 
			LIMIT 1`, mine.ID).Scan(&recurringCat, &recurringCount)

		stats := map[string]interface{}{
			"critical_violations":        critVal,
			"high_violations":            highVal,
			"medium_violations":          medVal,
			"low_violations":             lowVal,
			"overdue_actions":            overdueActions,
			"production_anomaly":         hasProdAnomaly,
			"attendance_anomaly":         hasAttAnomaly,
			"environmental_alerts":       envAlerts,
			"recurring_violations_count": recurringCount,
			"recurring_category":         recurringCat,
		}

		// JSON payload
		payload := map[string]interface{}{"stats": stats}
		jsonBytes, _ := json.Marshal(payload)

		// Call AI Flask service with normalized URL
		baseURL := strings.TrimRight(ac.Cfg.AIServiceURL, "/")
		aiURL := fmt.Sprintf("%s/predict-risk", baseURL)
		
		var score float64
		var classification string
		var factorsJSON []byte

		// Post HTTP request to python service
		client := &http.Client{Timeout: 2 * time.Second}
		resp, postErr := client.Post(aiURL, "application/json", bytes.NewBuffer(jsonBytes))
		
		if postErr == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			
			type aiResponse struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
				Data    struct {
					Score          float64       `json:"score"`
					Classification string        `json:"classification"`
					Factors        []interface{} `json:"factors"`
				} `json:"data"`
			}
			
			var aiRes aiResponse
			if err := json.Unmarshal(bodyBytes, &aiRes); err == nil && aiRes.Success {
				score = aiRes.Data.Score
				classification = aiRes.Data.Classification
				factorsJSON, _ = json.Marshal(aiRes.Data.Factors)
			}
		}

		// Fallback baseline calculation inside Go backend if AI Flask service is offline/unreachable
		if factorsJSON == nil {
			// Basic score
			rawScore := float64(critVal*25 + highVal*15 + medVal*8 + lowVal*3 + overdueActions*15)
			if hasProdAnomaly {
				rawScore += 12
			}
			if hasAttAnomaly {
				rawScore += 10
			}
			rawScore += float64(envAlerts * 10)
			if recurringCount > 0 {
				rawScore += 20
			}
			if rawScore > 100 {
				rawScore = 100
			}
			score = rawScore

			classification = "LOW"
			if score >= 81 {
				classification = "CRITICAL"
			} else if score >= 61 {
				classification = "HIGH"
			} else if score >= 31 {
				classification = "MEDIUM"
			}

			// Generate fallback factors
			fallbackFactors := []map[string]interface{}{}
			if critVal > 0 {
				fallbackFactors = append(fallbackFactors, map[string]interface{}{"name": strconv.Itoa(critVal) + " critical violations", "impact": critVal * 25})
			}
			if overdueActions > 0 {
				fallbackFactors = append(fallbackFactors, map[string]interface{}{"name": strconv.Itoa(overdueActions) + " overdue actions", "impact": overdueActions * 15})
			}
			if hasProdAnomaly {
				fallbackFactors = append(fallbackFactors, map[string]interface{}{"name": "Production anomaly drop", "impact": 12})
			}
			factorsJSON, _ = json.Marshal(fallbackFactors)
		}

		// Write to database
		_, _ = database.DB.Exec(`
			INSERT INTO risk_scores (mine_id, score, classification, factors_json) 
			VALUES (?, ?, ?, ?)`, 
			mine.ID, score, classification, string(factorsJSON))
	}
	return nil
}

type recurringViolationItem struct {
	MineID            int    `json:"mine_id"`
	MineName          string `json:"mine_name"`
	CategoryID        int    `json:"category_id"`
	CategoryName      string `json:"category_name"`
	TotalViolations   int    `json:"total_violations"`
	CriticalCount     int    `json:"critical_count"`
	HighCount         int    `json:"high_count"`
	MediumCount       int    `json:"medium_count"`
	OpenCount         int    `json:"open_count"`
	FirstDetected     string `json:"first_detected"`
	LatestDetected    string `json:"latest_detected"`
	RepeatLevel       string `json:"repeat_level"` // MODERATE, CHRONIC, SEVERE
	RiskScorePenalty  int    `json:"risk_score_penalty"`
}

// GetRecurringViolations analyzes violations grouped by mine and category/rule across a rolling time window.
func (ac *AnalyticsController) GetRecurringViolations(c *gin.Context) {
	windowDaysStr := c.DefaultQuery("window_days", "90")
	windowDays, _ := strconv.Atoi(windowDaysStr)
	if windowDays <= 0 {
		windowDays = 90
	}

	mineID := c.Query("mine_id")

	query := fmt.Sprintf(`
		SELECT v.mine_id, m.mine_name, v.category_id, cc.name AS category_name,
		       COUNT(*) AS total_violations,
		       SUM(CASE WHEN v.severity = 'CRITICAL' THEN 1 ELSE 0 END) AS crit_cnt,
		       SUM(CASE WHEN v.severity = 'HIGH' THEN 1 ELSE 0 END) AS high_cnt,
		       SUM(CASE WHEN v.severity = 'MEDIUM' THEN 1 ELSE 0 END) AS med_cnt,
		       SUM(CASE WHEN v.status IN ('OPEN', 'IN_PROGRESS', 'OVERDUE') THEN 1 ELSE 0 END) AS open_cnt,
		       MIN(v.created_at) AS first_date,
		       MAX(v.created_at) AS latest_date
		FROM violations v
		JOIN mines m ON m.id = v.mine_id
		JOIN compliance_categories cc ON cc.id = v.category_id
		WHERE v.created_at >= DATE_SUB(NOW(), INTERVAL %d DAY)`, windowDays)

	args := []interface{}{}
	if mineID != "" {
		query += " AND v.mine_id = ?"
		args = append(args, mineID)
	}

	query += `
		GROUP BY v.mine_id, m.mine_name, v.category_id, cc.name
		HAVING total_violations >= 2
		ORDER BY total_violations DESC, crit_cnt DESC`

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		utils.Fail(c, http.StatusInternalServerError, "Failed to analyze recurring violations", err.Error())
		return
	}
	defer rows.Close()

	list := []recurringViolationItem{}
	totalRepeatIncidents := 0
	chronicMineCount := make(map[int]bool)

	for rows.Next() {
		var it recurringViolationItem
		var firstDate, latestDate time.Time
		err := rows.Scan(
			&it.MineID, &it.MineName, &it.CategoryID, &it.CategoryName,
			&it.TotalViolations, &it.CriticalCount, &it.HighCount, &it.MediumCount, &it.OpenCount,
			&firstDate, &latestDate,
		)
		if err != nil {
			continue
		}

		it.FirstDetected = firstDate.Format("2006-01-02")
		it.LatestDetected = latestDate.Format("2006-01-02")

		// Determine severity level & AI risk penalty
		if it.TotalViolations >= 5 || it.CriticalCount >= 2 {
			it.RepeatLevel = "SEVERE"
			it.RiskScorePenalty = 30
		} else if it.TotalViolations >= 3 || it.CriticalCount >= 1 || it.HighCount >= 2 {
			it.RepeatLevel = "CHRONIC"
			it.RiskScorePenalty = 20
		} else {
			it.RepeatLevel = "MODERATE"
			it.RiskScorePenalty = 10
		}

		totalRepeatIncidents += it.TotalViolations
		chronicMineCount[it.MineID] = true
		list = append(list, it)
	}

	utils.Success(c, http.StatusOK, "Recurring violation patterns analyzed", gin.H{
		"window_days":            windowDays,
		"recurring_groups_count": len(list),
		"affected_mines_count":   len(chronicMineCount),
		"total_repeat_incidents": totalRepeatIncidents,
		"recurring_violations":   list,
	})
}


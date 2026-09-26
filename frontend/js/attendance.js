/**
 * attendance.js — Attendance & Workforce Management with Spoof-Resistant GPS Check-In.
 */
let allMines = [];
let allContractors = [];
let allWorkers = [];
let allAttendance = [];
let allAnomalies = [];

// Anti-Spoofing & Simulation State
let isMockLocationSimulated = false;
let clockSkewMinutes = 0;

// Real MediaPipe Facial Liveness State
let isLivenessVerified = false;
let livenessPassed = false;
let isLivenessRunning = false;
let livenessStep = 'IDLE'; // 'IDLE' | 'FACE' | 'BLINK' | 'LEFT' | 'RIGHT' | 'PASSED' | 'FAILED'
let faceMesh = null;
let cameraStream = null;
let animFrameId = null;
let challengeTimer = null;
let challengeSecondsRemaining = 40;
const CHALLENGE_TOTAL_SECONDS = 40;

// Landmark tracking buffers
let faceCenteredFrames = 0;
let blinkPhase = 'WAIT_CLOSE'; // 'WAIT_CLOSE' -> 'WAIT_OPEN'
let blinkTimestamp = 0;
let leftTurnFrames = 0;
let rightTurnFrames = 0;

document.addEventListener('DOMContentLoaded', async () => {
  AUTH.guardPage();
  AUTH.renderShell('attendance.html');

  const user = AUTH.getUser();
  const canManage = ['SUPER_ADMIN', 'MINE_MANAGER', 'SAFETY_OFFICER'].includes(user?.role_key);

  if (canManage) {
    const btnMark = document.getElementById('btn-mark-attendance');
    const btnAddWorker = document.getElementById('btn-add-worker');
    if (btnMark) btnMark.classList.remove('hidden');
    if (btnAddWorker) btnAddWorker.classList.remove('hidden');
  }

  // Set default date to today
  const todayStr = new Date().toISOString().split('T')[0];
  const filterDateEl = document.getElementById('filter-date');
  const attDateEl = document.getElementById('att-date');
  if (filterDateEl) filterDateEl.value = todayStr;
  if (attDateEl) attDateEl.value = todayStr;

  // Filter Event Listeners
  document.getElementById('filter-mine')?.addEventListener('change', applyFilters);
  document.getElementById('filter-contractor')?.addEventListener('change', applyFilters);
  document.getElementById('filter-date')?.addEventListener('change', applyFilters);
  document.getElementById('filter-status')?.addEventListener('change', applyFilters);

  // Standard Mark Attendance Modal
  const btnMark = document.getElementById('btn-mark-attendance');
  if (btnMark) btnMark.addEventListener('click', openMarkModal);
  document.getElementById('mark-modal-close')?.addEventListener('click', closeMarkModal);
  document.getElementById('mark-modal-cancel')?.addEventListener('click', closeMarkModal);
  document.getElementById('mark-form')?.addEventListener('submit', handleMarkSubmit);

  // Register Worker Modal
  const btnAddWorker = document.getElementById('btn-add-worker');
  if (btnAddWorker) btnAddWorker.addEventListener('click', openWorkerModal);
  document.getElementById('worker-modal-close')?.addEventListener('click', closeWorkerModal);
  document.getElementById('worker-modal-cancel')?.addEventListener('click', closeWorkerModal);
  document.getElementById('worker-form')?.addEventListener('submit', handleWorkerSubmit);

  document.getElementById('att-mine')?.addEventListener('change', (e) => populateWorkersDropdown(e.target.value, 'att-worker'));

  // Self Check-In Modal & Spoof-Resistance
  document.getElementById('btn-self-checkin')?.addEventListener('click', openSelfCheckinModal);
  document.getElementById('self-checkin-modal-close')?.addEventListener('click', closeSelfCheckinModal);
  document.getElementById('self-checkin-modal-cancel')?.addEventListener('click', closeSelfCheckinModal);
  document.getElementById('self-checkin-form')?.addEventListener('submit', handleSelfCheckinSubmit);

  document.getElementById('self-mine')?.addEventListener('change', (e) => {
    populateWorkersDropdown(e.target.value, 'self-worker');
    updateDistanceUI();
  });
  document.getElementById('self-lat')?.addEventListener('input', updateDistanceUI);
  document.getElementById('self-lng')?.addEventListener('input', updateDistanceUI);
  document.getElementById('btn-acquire-gps')?.addEventListener('click', acquireDeviceGPS);

  // Simulation Presets
  document.getElementById('sim-inside-geofence')?.addEventListener('click', simInsideGeofence);
  document.getElementById('sim-outside-geofence')?.addEventListener('click', simOutsideGeofence);
  document.getElementById('sim-mock-gps')?.addEventListener('click', simMockGPS);
  document.getElementById('sim-velocity-jump')?.addEventListener('click', simVelocityJump);
  document.getElementById('sim-clock-skew')?.addEventListener('click', simClockSkew);
  document.getElementById('sim-cluster-batch')?.addEventListener('click', simClusterBatch);

  // Liveness Controls
  document.getElementById('toggle-liveness')?.addEventListener('change', (e) => {
    document.getElementById('liveness-panel')?.classList.toggle('hidden', !e.target.checked);
    if (!e.target.checked) {
      stopLiveness();
    }
  });
  document.getElementById('btn-start-liveness')?.addEventListener('click', startLivenessChallenge);
  document.getElementById('btn-reset-liveness')?.addEventListener('click', resetLiveness);

  // Refresh Anomalies
  document.getElementById('btn-refresh-anomalies')?.addEventListener('click', loadAttendanceAnomalies);

  await loadInitialDropdowns();
  await loadAttendance();
  await loadReport();
  await loadAttendanceAnomalies();
});

/* =====================================================================
   Initial Data & Dropdowns
   ===================================================================== */
async function loadInitialDropdowns() {
  try {
    const [mines, contractors] = await Promise.all([
      API.get('/mines'),
      API.get('/contractors').catch(() => [])
    ]);
    allMines = mines || [];
    allContractors = contractors || [];

    // Populate Mine filters & modals
    const mineFilter = document.getElementById('filter-mine');
    const attMine = document.getElementById('att-mine');
    const wMine = document.getElementById('w-mine');
    const selfMine = document.getElementById('self-mine');

    const mineOpts = allMines.map(m => `<option value="${m.id}" data-lat="${m.latitude}" data-lng="${m.longitude}">${m.mine_name} (${m.mine_code})</option>`).join('');
    mineFilter.innerHTML = `<option value="">All Mines</option>${mineOpts}`;
    attMine.innerHTML = mineOpts;
    wMine.innerHTML = mineOpts;
    selfMine.innerHTML = mineOpts;

    // Populate Contractor filters & modals
    const contFilter = document.getElementById('filter-contractor');
    const wCont = document.getElementById('w-contractor');
    const contOpts = allContractors.map(c => `<option value="${c.id}">${c.company_name}</option>`).join('');
    contFilter.innerHTML = `<option value="">All Contractors / Direct</option>${contOpts}`;
    wCont.innerHTML = `<option value="">Direct CIL Employee</option>${contOpts}`;

    if (allMines.length > 0) {
      await populateWorkersDropdown(allMines[0].id, 'att-worker');
      await populateWorkersDropdown(allMines[0].id, 'self-worker');
      // Set default coords for self checkin modal
      if (allMines[0].latitude && allMines[0].longitude) {
        document.getElementById('self-lat').value = (allMines[0].latitude + 0.0002).toFixed(6);
        document.getElementById('self-lng').value = (allMines[0].longitude + 0.0002).toFixed(6);
      }
    }
  } catch (err) {
    console.error('Failed to load initial dropdowns', err);
  }
}

async function populateWorkersDropdown(mineId, targetElementId) {
  try {
    const workers = await API.get(`/workers?mine_id=${mineId}`);
    const selectElem = document.getElementById(targetElementId);
    if (!selectElem) return;

    if (!workers || workers.length === 0) {
      selectElem.innerHTML = `<option value="">No active workers registered</option>`;
    } else {
      selectElem.innerHTML = workers.map(w => `<option value="${w.id}">${w.full_name} (${w.worker_code} - ${w.designation})</option>`).join('');
    }
  } catch (e) {
    console.error('Failed to populate workers', e);
  }
}

/* =====================================================================
   Roster & Reports
   ===================================================================== */
async function loadAttendance() {
  const tbody = document.getElementById('attendance-table-body');
  tbody.innerHTML = `<tr><td colspan="12" class="state-panel">Loading attendance roster...</td></tr>`;

  try {
    const mineId = document.getElementById('filter-mine').value;
    const contId = document.getElementById('filter-contractor').value;
    const date = document.getElementById('filter-date').value;
    const status = document.getElementById('filter-status').value;

    let url = `/attendance?`;
    if (mineId) url += `mine_id=${mineId}&`;
    if (contId) url += `contractor_id=${contId}&`;
    if (date) url += `date=${date}&`;
    if (status) url += `status=${status}&`;

    allAttendance = await API.get(url);
    renderAttendanceTable(allAttendance);
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="12" class="state-panel error">${err.message}</td></tr>`;
  }
}

async function loadReport() {
  try {
    const mineId = document.getElementById('filter-mine').value;
    const contId = document.getElementById('filter-contractor').value;
    let url = `/attendance/report?`;
    if (mineId) url += `mine_id=${mineId}&`;
    if (contId) url += `contractor_id=${contId}&`;

    const report = await API.get(url);
    document.getElementById('kpi-total-workers').textContent = report.total_workers || '0';
    document.getElementById('kpi-present-count').textContent = report.status_breakdown?.PRESENT || '0';
    
    const absentTotal = (report.status_breakdown?.ABSENT || 0) + (report.status_breakdown?.LEAVE || 0);
    document.getElementById('kpi-absent-count').textContent = absentTotal;
    
    const rate = Math.round(report.attendance_rate || 0);
    document.getElementById('kpi-rate').textContent = `${rate}%`;
  } catch (e) {
    console.error('Failed to load attendance report', e);
  }
}

function applyFilters() {
  loadAttendance();
  loadReport();
}

function renderAttendanceTable(records) {
  const tbody = document.getElementById('attendance-table-body');
  if (!records || records.length === 0) {
    tbody.innerHTML = `<tr><td colspan="12" class="state-panel">No attendance records match current filters.</td></tr>`;
    return;
  }

  tbody.innerHTML = records.map(r => {
    let badgeClass = 'badge-active';
    if (r.status === 'ABSENT') badgeClass = 'badge-critical';
    else if (r.status === 'LEAVE') badgeClass = 'badge-info';
    else if (r.status === 'HALF_DAY') badgeClass = 'badge-warning';

    // Verification & Geolocation Tag
    let geoHtml = `<span class="geo-tag manual">📋 Manager Batch</span>`;
    if (r.distance_from_mine_m !== null && r.distance_from_mine_m !== undefined) {
      const dist = Math.round(r.distance_from_mine_m);
      if (r.is_mock_location) {
        geoHtml = `<span class="geo-tag mock">🚫 Mock GPS (${dist}m)</span>`;
      } else if (dist <= 500) {
        geoHtml = `<span class="geo-tag verified">📍 Verified (${dist}m)</span>`;
      } else {
        geoHtml = `<span class="geo-tag breach">⚠️ Out of Bounds (${dist}m)</span>`;
      }
    }

    // Tamper Flag Tag
    let tamperHtml = `<span class="badge" style="background:#f1f5f9; color:#64748b;">Standard</span>`;
    if (r.tamper_flag) {
      tamperHtml = `<span class="badge badge-critical" style="font-weight:600;">🚨 Flagged</span>`;
    } else if (r.distance_from_mine_m !== null && r.distance_from_mine_m !== undefined) {
      tamperHtml = `<span class="badge badge-active">🛡️ Secure</span>`;
    }

    return `
      <tr>
        <td class="mono font-semibold">${r.worker_code || 'AGGREGATE'}</td>
        <td><strong>${r.worker_name || 'Mine-wide Headcount'}</strong></td>
        <td>${r.designation || 'All Shifts'}</td>
        <td>${r.mine_name}</td>
        <td>${r.contractor_name || '<span class="text-muted">CIL Direct</span>'}</td>
        <td><span class="mono">${r.shift || 'GENERAL'}</span></td>
        <td>${r.overtime_hours > 0 ? `<strong>+${r.overtime_hours} hrs</strong>` : '-'}</td>
        <td class="mono">${r.record_date}</td>
        <td><span class="badge ${badgeClass}">${r.status}</span></td>
        <td>${geoHtml}</td>
        <td>${tamperHtml}</td>
        <td>${r.marked_by_name || 'Self Check-In'}</td>
      </tr>
    `;
  }).join('');
}

/* =====================================================================
   Anti-Spoofing & Tamper Audit Log
   ===================================================================== */
async function loadAttendanceAnomalies() {
  const tbody = document.getElementById('anomalies-table-body');
  if (!tbody) return;

  try {
    const res = await API.get('/analytics/anomalies');
    const anomalies = Array.isArray(res) ? res : (res?.data || []);
    
    // Filter attendance and spoofing-related anomalies
    const attAnomalies = anomalies.filter(a => 
      ['MOCK_LOCATION', 'GEOFENCE_BREACH', 'TIME_ANOMALY', 'VELOCITY_ANOMALY', 'SCRIPTED_BATCH', 'LIVENESS_FAILED'].includes(a.anomaly_type) ||
      a.worker_id !== null && a.worker_id !== undefined
    );

    if (attAnomalies.length === 0) {
      tbody.innerHTML = `<tr><td colspan="7" class="state-panel" style="color:var(--color-low);">No tamper anomalies detected. All attendance check-ins are verified and secure.</td></tr>`;
      return;
    }

    tbody.innerHTML = attAnomalies.map(a => {
      let sevClass = 'badge-critical';
      if (a.severity === 'HIGH') sevClass = 'badge-warning';
      else if (a.severity === 'MEDIUM') sevClass = 'badge-info';
      else if (a.severity === 'LOW') sevClass = 'badge-active';

      let timeFormatted = a.detected_at ? new Date(a.detected_at).toLocaleString('en-IN', { timeZone: 'Asia/Kolkata' }) : '-';

      let vectorLabel = a.anomaly_type;
      if (a.anomaly_type === 'MOCK_LOCATION') vectorLabel = '🚫 Mock GPS Provider';
      else if (a.anomaly_type === 'GEOFENCE_BREACH') vectorLabel = '⚠️ Geofence Perimeter Breach';
      else if (a.anomaly_type === 'VELOCITY_ANOMALY') vectorLabel = '⚡ Velocity Jump (>80 km/h)';
      else if (a.anomaly_type === 'SCRIPTED_BATCH') vectorLabel = '🤖 Scripted Cluster Batch';
      else if (a.anomaly_type === 'TIME_ANOMALY') vectorLabel = '🕒 Clock Skew Tampering';
      else if (a.anomaly_type === 'LIVENESS_FAILED') vectorLabel = '👁️ Liveness Test Mismatch';

      return `
        <tr>
          <td class="mono font-semibold" style="font-size:12px;">${timeFormatted}</td>
          <td><strong>${a.worker_name ? `${a.worker_name} (${a.worker_code})` : `Worker #${a.worker_id || '-'}`}</strong></td>
          <td>${a.mine_name || 'Mine'}</td>
          <td><span class="mono font-semibold" style="font-size:12px;">${vectorLabel}</span></td>
          <td><span class="badge ${sevClass}">${a.severity}</span></td>
          <td style="font-size:12.5px; color:var(--color-ink); max-width:320px;">${a.description}</td>
          <td><span class="badge badge-warning">${a.status}</span></td>
        </tr>
      `;
    }).join('');
  } catch (err) {
    console.error('Failed to load anomalies audit log', err);
    tbody.innerHTML = `<tr><td colspan="7" class="state-panel error">Failed to load audit log: ${err.message}</td></tr>`;
  }
}

/* =====================================================================
   Self Check-In Modal & Anti-Spoofing Actions
   ===================================================================== */
function openSelfCheckinModal() {
  document.getElementById('self-checkin-modal').classList.remove('hidden');
  document.getElementById('self-checkin-alert').classList.add('hidden');
  document.getElementById('self-checkin-success').classList.add('hidden');
  
  // Reset liveness UI for clean session
  resetLivenessUI();

  // Default mine & workers
  const selMine = document.getElementById('self-mine');
  if (selMine && selMine.value) {
    populateWorkersDropdown(selMine.value, 'self-worker');
    updateDistanceUI();
  }
}

function closeSelfCheckinModal() {
  document.getElementById('self-checkin-modal').classList.add('hidden');
  document.getElementById('self-checkin-alert').classList.add('hidden');
  document.getElementById('self-checkin-success').classList.add('hidden');
  isMockLocationSimulated = false;
  clockSkewMinutes = 0;
  stopLiveness();
}

function computeHaversine(lat1, lon1, lat2, lon2) {
  const R = 6371000; // meters
  const dLat = (lat2 - lat1) * Math.PI / 180;
  const dLon = (lon2 - lon1) * Math.PI / 180;
  const a = Math.sin(dLat / 2) * Math.sin(dLat / 2) +
            Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
            Math.sin(dLon / 2) * Math.sin(dLon / 2);
  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  return R * c;
}

function updateDistanceUI() {
  const mineId = parseInt(document.getElementById('self-mine').value);
  const lat = parseFloat(document.getElementById('self-lat').value);
  const lng = parseFloat(document.getElementById('self-lng').value);

  const distVal = document.getElementById('dist-calc-value');
  const tag = document.getElementById('geofence-status-tag');
  const box = document.getElementById('distance-indicator-box');

  const mine = allMines.find(m => m.id === mineId);
  if (!mine || isNaN(lat) || isNaN(lng) || !mine.latitude || !mine.longitude) {
    distVal.textContent = '-- m';
    tag.className = 'geo-tag manual';
    tag.textContent = 'Awaiting GPS Coordinates';
    box.className = 'distance-indicator';
    return;
  }

  const distanceM = computeHaversine(lat, lng, mine.latitude, mine.longitude);
  distVal.textContent = `${Math.round(distanceM)} meters`;

  if (distanceM <= 500) {
    tag.className = 'geo-tag verified';
    tag.innerHTML = `✓ Within 500m Geofence (${Math.round(distanceM)}m)`;
    box.className = 'distance-indicator ok';
  } else {
    tag.className = 'geo-tag breach';
    tag.innerHTML = `⚠️ Outside Geofence (${(distanceM / 1000).toFixed(2)}km &gt; 500m)`;
    box.className = 'distance-indicator out-of-bounds';
  }
}

function acquireDeviceGPS() {
  const statusText = document.getElementById('gps-status-text');
  if (!navigator.geolocation) {
    statusText.textContent = 'Geolocation is not supported by your browser';
    return;
  }

  statusText.textContent = 'Acquiring GPS fix...';
  navigator.geolocation.getCurrentPosition(
    (pos) => {
      document.getElementById('self-lat').value = pos.coords.latitude.toFixed(6);
      document.getElementById('self-lng').value = pos.coords.longitude.toFixed(6);
      statusText.textContent = `GPS Acquired (Accuracy: ±${Math.round(pos.coords.accuracy)}m)`;
      isMockLocationSimulated = false;
      clockSkewMinutes = 0;
      updateDistanceUI();
    },
    (err) => {
      statusText.textContent = `GPS Error: ${err.message}`;
    },
    { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
  );
}

/* =====================================================================
   Simulation Presets (For Demonstration / Evaluation)
   ===================================================================== */
function simInsideGeofence() {
  const mineId = parseInt(document.getElementById('self-mine').value);
  const mine = allMines.find(m => m.id === mineId) || allMines[0];
  if (mine && mine.latitude && mine.longitude) {
    document.getElementById('self-lat').value = (mine.latitude + 0.0003).toFixed(6);
    document.getElementById('self-lng').value = (mine.longitude + 0.0003).toFixed(6);
  }
  isMockLocationSimulated = false;
  clockSkewMinutes = 0;
  document.getElementById('gps-status-text').textContent = 'Simulated: Inside 500m Geofence (~45m from mine center)';
  updateDistanceUI();
}

function simOutsideGeofence() {
  const mineId = parseInt(document.getElementById('self-mine').value);
  const mine = allMines.find(m => m.id === mineId) || allMines[0];
  if (mine && mine.latitude && mine.longitude) {
    document.getElementById('self-lat').value = (mine.latitude + 0.025).toFixed(6);
    document.getElementById('self-lng').value = (mine.longitude + 0.025).toFixed(6);
  }
  isMockLocationSimulated = false;
  clockSkewMinutes = 0;
  document.getElementById('gps-status-text').textContent = 'Simulated: Outside Geofence (2.8 km away - Breach Test)';
  updateDistanceUI();
}

function simMockGPS() {
  simInsideGeofence();
  isMockLocationSimulated = true;
  document.getElementById('gps-status-text').innerHTML = '<span style="color:#b45309; font-weight:600;">⚠️ Simulated Mock GPS Provider Active (isFromMockProvider: true)</span>';
}

function simVelocityJump() {
  const mineId = parseInt(document.getElementById('self-mine').value);
  const mine = allMines.find(m => m.id === mineId) || allMines[0];
  if (mine && mine.latitude && mine.longitude) {
    document.getElementById('self-lat').value = (mine.latitude + 1.2).toFixed(6);
    document.getElementById('self-lng').value = (mine.longitude + 1.2).toFixed(6);
  }
  isMockLocationSimulated = false;
  clockSkewMinutes = 0;
  document.getElementById('gps-status-text').textContent = 'Simulated: Teleport Jump (~150km away within minutes)';
  updateDistanceUI();
}

function simClockSkew() {
  simInsideGeofence();
  clockSkewMinutes = -25; // 25 mins in past
  document.getElementById('gps-status-text').textContent = 'Simulated: Client Clock manipulated (-25 min skew)';
}

async function simClusterBatch() {
  const mineId = parseInt(document.getElementById('self-mine').value);
  const mine = allMines.find(m => m.id === mineId) || allMines[0];
  if (!mine) return;

  const lat = (mine.latitude + 0.0001);
  const lng = (mine.longitude + 0.0001);
  document.getElementById('gps-status-text').textContent = 'Firing 5 rapid check-ins in parallel to simulate scripted cluster attack...';

  try {
    const workers = await API.get(`/workers?mine_id=${mineId}`);
    if (workers.length < 5) {
      showToast('Need at least 5 registered workers in mine to demonstrate cluster attack', 'warning');
      return;
    }

    const promises = workers.slice(0, 5).map(w => {
      return API.post('/attendance/self-checkin', {
        mine_id: mineId,
        worker_id: w.id,
        lat: lat,
        lng: lng,
        is_mock_location: false,
        device_uptime_ms: Math.floor(performance.now()),
        client_reported_time: new Date().toISOString(),
        liveness_passed: true
      }).catch(err => ({ error: err.message }));
    });

    await Promise.all(promises);
    showToast('Scripted batch attack sent. Checking audit anomalies...', 'info');
    await loadAttendance();
    await loadAttendanceAnomalies();
    document.getElementById('gps-status-text').textContent = 'Cluster test completed. Check Anti-Spoofing Audit Log below!';
  } catch (err) {
    showError(err);
  }
}

/* =====================================================================
   Real Browser-Based Facial Liveness Detection (MediaPipe FaceMesh)
   ===================================================================== */

function resetLivenessUI() {
  stopLiveness();
  isLivenessVerified = false;
  livenessPassed = false;
  livenessStep = 'IDLE';

  const stepPills = ['step-pill-face', 'step-pill-blink', 'step-pill-left', 'step-pill-right'];
  stepPills.forEach(id => {
    const el = document.getElementById(id);
    if (el) el.className = 'liveness-step-pill';
  });

  const prompt = document.getElementById('liveness-prompt');
  if (prompt) {
    prompt.textContent = 'Click "Start Liveness Verification" to open camera';
    prompt.style.color = '#38bdf8';
  }

  const oval = document.getElementById('liveness-oval');
  if (oval) oval.className = 'liveness-oval-guide';

  const btnStart = document.getElementById('btn-start-liveness');
  const btnReset = document.getElementById('btn-reset-liveness');
  if (btnStart) btnStart.style.display = 'inline-block';
  if (btnReset) btnReset.style.display = 'none';

  updateMetricDisplays('--', '--', 'None', 'Ready');
  updateTimerBar(100, '#38bdf8');
}

function resetLiveness() {
  resetLivenessUI();
  startLivenessChallenge();
}

async function startLivenessChallenge() {
  resetLivenessUI();

  const prompt = document.getElementById('liveness-prompt');
  const video = document.getElementById('liveness-video');
  const canvas = document.getElementById('liveness-canvas');
  const btnStart = document.getElementById('btn-start-liveness');
  const btnReset = document.getElementById('btn-reset-liveness');

  if (typeof FaceMesh === 'undefined') {
    if (prompt) prompt.textContent = '⚠️ MediaPipe FaceMesh library loading... Please check connection.';
    return;
  }

  try {
    if (prompt) prompt.textContent = 'Requesting camera permission...';
    if (btnStart) btnStart.style.display = 'none';

    cameraStream = await navigator.mediaDevices.getUserMedia({
      video: {
        width: { ideal: 640 },
        height: { ideal: 480 },
        facingMode: 'user'
      }
    });

    video.srcObject = cameraStream;
    await video.play();

    // Initialize FaceMesh instance once
    if (!faceMesh) {
      if (prompt) prompt.textContent = 'Loading liveness model...';
      faceMesh = new FaceMesh({
        locateFile: (file) => `https://cdn.jsdelivr.net/npm/@mediapipe/face_mesh/${file}`
      });
      faceMesh.setOptions({
        maxNumFaces: 2,
        refineLandmarks: true,
        minDetectionConfidence: 0.5,
        minTrackingConfidence: 0.5
      });
      faceMesh.onResults(onFaceResults);
    }

    isLivenessRunning = true;
    livenessStep = 'FACE';
    faceCenteredFrames = 0;
    blinkPhase = 'WAIT_CLOSE';
    leftTurnFrames = 0;
    rightTurnFrames = 0;

    // Activate Step 1
    const pill1 = document.getElementById('step-pill-face');
    if (pill1) pill1.className = 'liveness-step-pill active';
    if (prompt) {
      prompt.textContent = 'Position your face inside the frame';
      prompt.style.color = '#38bdf8';
    }
    updateMetricDisplays('--', '--', 'Searching', 'Face Step');

    // Start 40-second countdown timer
    startChallengeTimer();

    // Begin frame analysis loop
    requestAnimationFrame(processLivenessFrame);
  } catch (err) {
    console.error('Camera access or FaceMesh error:', err);
    isLivenessRunning = false;
    if (btnReset) btnReset.style.display = 'inline-block';
    if (prompt) {
      prompt.textContent = '⚠️ Camera permission is required for attendance verification.';
      prompt.style.color = '#ef4444';
    }
    const oval = document.getElementById('liveness-oval');
    if (oval) oval.className = 'liveness-oval-guide danger';
    updateMetricDisplays('--', '--', 'Error', 'Permission Denied');
  }
}

async function processLivenessFrame() {
  if (!isLivenessRunning) return;

  const video = document.getElementById('liveness-video');
  if (video && video.readyState >= 2 && faceMesh) {
    try {
      await faceMesh.send({ image: video });
    } catch (e) {
      console.warn('FaceMesh frame send error:', e);
    }
  }

  if (isLivenessRunning) {
    animFrameId = requestAnimationFrame(processLivenessFrame);
  }
}

function onFaceResults(results) {
  if (!isLivenessRunning) return;

  const video = document.getElementById('liveness-video');
  const canvas = document.getElementById('liveness-canvas');
  if (!canvas || !video) return;

  const ctx = canvas.getContext('2d');
  canvas.width = video.videoWidth || 340;
  canvas.height = video.videoHeight || 230;
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  const prompt = document.getElementById('liveness-prompt');
  const oval = document.getElementById('liveness-oval');

  // 1. Handle No Face Detected
  if (!results || !results.multiFaceLandmarks || results.multiFaceLandmarks.length === 0) {
    if (livenessStep !== 'PASSED') {
      faceCenteredFrames = 0;
      if (oval) oval.className = 'liveness-oval-guide';
      if (prompt && livenessStep === 'FACE') {
        prompt.textContent = 'Face not detected. Please position yourself in front of the camera.';
        prompt.style.color = '#38bdf8';
      }
      updateMetricDisplays('--', '--', 'None', 'Searching');
    }
    return;
  }

  // 2. Handle Multiple Faces Detected (Anti-Spoofing / Multi-Person Rejection)
  if (results.multiFaceLandmarks.length > 1) {
    if (livenessStep !== 'PASSED') {
      if (oval) oval.className = 'liveness-oval-guide danger';
      if (prompt) {
        prompt.textContent = 'Multiple faces detected. Only the worker should be visible.';
        prompt.style.color = '#ef4444';
      }
      updateMetricDisplays('--', '--', 'Multiple (2+)', 'Rejected');
    }
    return;
  }

  // Exactly 1 Face Detected
  const landmarks = results.multiFaceLandmarks[0];

  // Draw subtle face mesh & eye landmark dots on canvas
  drawLandmarkOverlay(ctx, canvas, landmarks);

  // 3. Compute Centering & Bounding Box
  let minX = 1, maxX = 0, minY = 1, maxY = 0;
  for (let i = 0; i < landmarks.length; i++) {
    const p = landmarks[i];
    if (p.x < minX) minX = p.x;
    if (p.x > maxX) maxX = p.x;
    if (p.y < minY) minY = p.y;
    if (p.y > maxY) maxY = p.y;
  }
  const faceWidth = maxX - minX;
  const faceHeight = maxY - minY;
  const centerX = (minX + maxX) / 2;
  const centerY = (minY + maxY) / 2;
  const isCentered = (centerX >= 0.20 && centerX <= 0.80 && centerY >= 0.18 && centerY <= 0.82 && faceWidth >= 0.18 && faceWidth <= 0.85);

  // 4. Compute Eye Aspect Ratio (EAR) for Blink Detection
  const dist2d = (i1, i2) => Math.hypot(landmarks[i1].x - landmarks[i2].x, landmarks[i1].y - landmarks[i2].y);

  // Left Eye (Landmarks: 160-144, 159-145, Corners: 33-133)
  const leftV1 = dist2d(160, 144);
  const leftV2 = dist2d(159, 145);
  const leftH = dist2d(33, 133);
  const leftEAR = (leftV1 + leftV2) / (2.0 * Math.max(leftH, 0.001));

  // Right Eye (Landmarks: 385-380, 386-374, Corners: 362-263)
  const rightV1 = dist2d(385, 380);
  const rightV2 = dist2d(386, 374);
  const rightH = dist2d(362, 263);
  const rightEAR = (rightV1 + rightV2) / (2.0 * Math.max(rightH, 0.001));

  const avgEAR = (leftEAR + rightEAR) / 2.0;

  // 5. Compute Head Yaw (Horizontal Rotation)
  // Nose tip (1), Left cheek boundary (234), Right cheek boundary (454)
  const nose = landmarks[1];
  const leftCheek = landmarks[234];
  const rightCheek = landmarks[454];
  const distLeft = Math.abs(nose.x - leftCheek.x);
  const distRight = Math.abs(rightCheek.x - nose.x);
  const yawRatio = distLeft / Math.max(distLeft + distRight, 0.001);
  const noseCenterOffset = (nose.x - (leftCheek.x + rightCheek.x) / 2) / Math.max(distLeft + distRight, 0.001);

  // Update dynamic metrics display
  updateMetricDisplays(avgEAR.toFixed(2), yawRatio.toFixed(2), isCentered ? 'Centered ✓' : 'Off-center', livenessStep);

  // 6. Challenge State Machine
  if (livenessStep === 'FACE') {
    if (isCentered) {
      faceCenteredFrames++;
      if (oval) oval.className = 'liveness-oval-guide warning';
      if (prompt) prompt.textContent = 'Face positioned! Hold steady...';
      if (faceCenteredFrames >= 8) {
        // Step 1 Passed
        setStepCompleted('step-pill-face');
        setStepActive('step-pill-blink');
        if (oval) oval.className = 'liveness-oval-guide success';
        if (prompt) {
          prompt.textContent = 'Face detected ✓ Please blink once';
          prompt.style.color = '#38bdf8';
        }
        livenessStep = 'BLINK';
        blinkPhase = 'WAIT_CLOSE';
      }
    } else {
      faceCenteredFrames = Math.max(0, faceCenteredFrames - 1);
      if (oval) oval.className = 'liveness-oval-guide';
      if (prompt) prompt.textContent = 'Position your face inside the frame';
    }
  } else if (livenessStep === 'BLINK') {
    if (blinkPhase === 'WAIT_CLOSE') {
      if (avgEAR < 0.17) {
        blinkPhase = 'WAIT_OPEN';
        blinkTimestamp = performance.now();
        if (prompt) prompt.textContent = 'Eyes closing... now open your eyes';
      }
    } else if (blinkPhase === 'WAIT_OPEN') {
      if (avgEAR > 0.22 && (performance.now() - blinkTimestamp < 2500)) {
        // Step 2 Passed
        setStepCompleted('step-pill-blink');
        setStepActive('step-pill-left');
        if (prompt) {
          prompt.textContent = 'Blink detected ✓ Turn your head LEFT';
          prompt.style.color = '#38bdf8';
        }
        livenessStep = 'LEFT';
        leftTurnFrames = 0;
      }
    }
  } else if (livenessStep === 'LEFT') {
    // Left turn detection (yawRatio > 0.58 or nose offset > 0.08)
    if (yawRatio > 0.58 || noseCenterOffset > 0.08) {
      leftTurnFrames++;
      if (prompt) prompt.textContent = 'Turning left detected... hold';
      if (leftTurnFrames >= 4) {
        // Step 3 Passed
        setStepCompleted('step-pill-left');
        setStepActive('step-pill-right');
        if (prompt) {
          prompt.textContent = 'Left movement detected ✓ Turn your head RIGHT';
          prompt.style.color = '#38bdf8';
        }
        livenessStep = 'RIGHT';
        rightTurnFrames = 0;
      }
    }
  } else if (livenessStep === 'RIGHT') {
    // Right turn detection (yawRatio < 0.42 or nose offset < -0.08)
    if (yawRatio < 0.42 || noseCenterOffset < -0.08) {
      rightTurnFrames++;
      if (prompt) prompt.textContent = 'Turning right detected... hold';
      if (rightTurnFrames >= 4) {
        // Step 4 Passed
        setStepCompleted('step-pill-right');
        livenessStep = 'PASSED';
        handleLivenessSuccess();
      }
    }
  }
}

function drawLandmarkOverlay(ctx, canvas, landmarks) {
  const w = canvas.width;
  const h = canvas.height;

  // Draw eye landmarks
  const eyeIndices = [33, 133, 159, 145, 160, 144, 362, 263, 386, 374, 385, 380];
  ctx.fillStyle = '#38bdf8';
  for (const idx of eyeIndices) {
    const pt = landmarks[idx];
    if (pt) {
      ctx.beginPath();
      ctx.arc(pt.x * w, pt.y * h, 2, 0, 2 * Math.PI);
      ctx.fill();
    }
  }

  // Draw nose tip
  const nose = landmarks[1];
  if (nose) {
    ctx.fillStyle = '#10b981';
    ctx.beginPath();
    ctx.arc(nose.x * w, nose.y * h, 3.5, 0, 2 * Math.PI);
    ctx.fill();
  }

  // Draw face contour boundary points
  const contourIndices = [10, 338, 297, 332, 284, 251, 389, 356, 454, 323, 361, 288, 397, 365, 379, 378, 400, 377, 152, 148, 176, 149, 150, 136, 172, 58, 132, 93, 234, 127, 162, 21, 54, 103, 67, 109];
  ctx.strokeStyle = 'rgba(56, 189, 248, 0.35)';
  ctx.lineWidth = 1;
  ctx.beginPath();
  let first = true;
  for (const idx of contourIndices) {
    const pt = landmarks[idx];
    if (pt) {
      if (first) {
        ctx.moveTo(pt.x * w, pt.y * h);
        first = false;
      } else {
        ctx.lineTo(pt.x * w, pt.y * h);
      }
    }
  }
  ctx.closePath();
  ctx.stroke();
}

function setStepActive(pillId) {
  const el = document.getElementById(pillId);
  if (el) el.className = 'liveness-step-pill active';
}

function setStepCompleted(pillId) {
  const el = document.getElementById(pillId);
  if (el) el.className = 'liveness-step-pill completed';
}

function updateMetricDisplays(ear, yaw, face, status) {
  const elEar = document.getElementById('metric-ear');
  const elYaw = document.getElementById('metric-yaw');
  const elFace = document.getElementById('metric-face');
  const elStatus = document.getElementById('metric-status');

  if (elEar) elEar.textContent = ear;
  if (elYaw) elYaw.textContent = yaw;
  if (elFace) elFace.textContent = face;
  if (elStatus) elStatus.textContent = status;
}

function startChallengeTimer() {
  clearInterval(challengeTimer);
  challengeSecondsRemaining = CHALLENGE_TOTAL_SECONDS;
  updateTimerBar(100, '#38bdf8');

  challengeTimer = setInterval(() => {
    challengeSecondsRemaining--;
    const percent = Math.max(0, (challengeSecondsRemaining / CHALLENGE_TOTAL_SECONDS) * 100);
    const barColor = challengeSecondsRemaining <= 10 ? '#ef4444' : '#38bdf8';
    updateTimerBar(percent, barColor);

    if (challengeSecondsRemaining <= 0) {
      clearInterval(challengeTimer);
      handleLivenessFailure('Liveness verification timed out. Please try again.');
    }
  }, 1000);
}

function updateTimerBar(percent, color) {
  const bar = document.getElementById('liveness-timer-bar');
  if (bar) {
    bar.style.width = `${percent}%`;
    bar.style.backgroundColor = color;
  }
}

function handleLivenessSuccess() {
  isLivenessVerified = true;
  livenessPassed = true;
  clearInterval(challengeTimer);

  updateTimerBar(100, '#10b981');

  const prompt = document.getElementById('liveness-prompt');
  if (prompt) {
    prompt.textContent = '✅ Liveness verification successful! Ready to check in.';
    prompt.style.color = '#10b981';
  }

  const oval = document.getElementById('liveness-oval');
  if (oval) oval.className = 'liveness-oval-guide success';

  updateMetricDisplays('--', '--', 'Verified ✓', 'PASSED');

  const btnStart = document.getElementById('btn-start-liveness');
  const btnReset = document.getElementById('btn-reset-liveness');
  if (btnStart) btnStart.style.display = 'none';
  if (btnReset) {
    btnReset.style.display = 'inline-block';
    btnReset.textContent = '🔄 Test Liveness Again';
  }

  showToast('Facial liveness verified successfully', 'success');

  // Stop camera tracks after 1.5s to preserve CPU
  setTimeout(() => {
    if (cameraStream) {
      cameraStream.getTracks().forEach(t => t.stop());
      cameraStream = null;
    }
    isLivenessRunning = false;
    cancelAnimationFrame(animFrameId);
  }, 1500);
}

function handleLivenessFailure(reason) {
  isLivenessVerified = false;
  livenessPassed = false;
  livenessStep = 'FAILED';
  clearInterval(challengeTimer);
  updateTimerBar(100, '#ef4444');

  const prompt = document.getElementById('liveness-prompt');
  if (prompt) {
    prompt.textContent = `⚠️ ${reason}`;
    prompt.style.color = '#ef4444';
  }

  const oval = document.getElementById('liveness-oval');
  if (oval) oval.className = 'liveness-oval-guide danger';

  updateMetricDisplays('--', '--', 'Failed', 'FAILED');

  const btnStart = document.getElementById('btn-start-liveness');
  const btnReset = document.getElementById('btn-reset-liveness');
  if (btnStart) btnStart.style.display = 'none';
  if (btnReset) {
    btnReset.style.display = 'inline-block';
    btnReset.textContent = '🔄 Retry Challenge';
  }

  stopLiveness();
}

function stopLiveness() {
  isLivenessRunning = false;
  clearInterval(challengeTimer);
  if (animFrameId) {
    cancelAnimationFrame(animFrameId);
    animFrameId = null;
  }
  if (cameraStream) {
    cameraStream.getTracks().forEach(t => t.stop());
    cameraStream = null;
  }
  const video = document.getElementById('liveness-video');
  if (video) video.srcObject = null;
  const canvas = document.getElementById('liveness-canvas');
  if (canvas) {
    const ctx = canvas.getContext('2d');
    ctx.clearRect(0, 0, canvas.width, canvas.height);
  }
}

/* =====================================================================
   Handle Self Check-In Form Submit
   ===================================================================== */
async function handleSelfCheckinSubmit(e) {
  e.preventDefault();
  const alertBox = document.getElementById('self-checkin-alert');
  const alertText = document.getElementById('self-checkin-alert-text');
  const successBox = document.getElementById('self-checkin-success');
  const successText = document.getElementById('self-checkin-success-text');

  alertBox.classList.add('hidden');
  successBox.classList.add('hidden');

  const mineId = parseInt(document.getElementById('self-mine').value);
  const workerId = parseInt(document.getElementById('self-worker').value);
  const lat = parseFloat(document.getElementById('self-lat').value);
  const lng = parseFloat(document.getElementById('self-lng').value);
  const toggleLiveness = document.getElementById('toggle-liveness').checked;

  if (isNaN(mineId) || isNaN(workerId)) {
    alertText.textContent = 'Please select a valid Mine Site and Worker.';
    alertBox.classList.remove('hidden');
    return;
  }

  if (isNaN(lat) || isNaN(lng)) {
    alertText.textContent = 'Please provide valid GPS latitude and longitude coordinates.';
    alertBox.classList.remove('hidden');
    return;
  }

  // Calculate client time with any simulated skew
  const clientTime = new Date(Date.now() + clockSkewMinutes * 60000).toISOString();

  const payload = {
    mine_id: mineId,
    worker_id: workerId,
    lat: lat,
    lng: lng,
    is_mock_location: isMockLocationSimulated,
    device_uptime_ms: Math.floor(performance.now()),
    client_reported_time: clientTime,
    liveness_passed: toggleLiveness ? isLivenessVerified : null
  };

  const submitBtn = document.getElementById('self-checkin-submit-btn');
  submitBtn.disabled = true;
  submitBtn.textContent = 'Verifying with Anti-Spoofing Engine...';

  try {
    const res = await API.post('/attendance/self-checkin', payload);
    const data = res?.data || res;

    successText.innerHTML = `
      <strong>Check-In Confirmed!</strong><br />
      Distance to mine center: <strong>${Math.round(data.distance_from_mine_m || 0)}m</strong> (Geofence limit: ${data.geofence_radius_m || 500}m).<br />
      Tamper Flag: <strong>${data.tamper_flag ? 'Flagged for Audit' : 'Secure'}</strong> | Liveness: <strong>${data.liveness_passed ? 'Verified' : 'Bypassed / Failed'}</strong>
    `;
    successBox.classList.remove('hidden');
    showToast('Worker check-in verified and recorded successfully', 'success');

    await loadAttendance();
    await loadReport();
    await loadAttendanceAnomalies();

    setTimeout(() => {
      closeSelfCheckinModal();
    }, 2500);
  } catch (err) {
    // 403 Forbidden or 400 Bad Request
    alertText.innerHTML = `
      <strong>Check-In Blocked by Anti-Spoofing Policy:</strong><br />
      ${err.message || 'Verification rejected'}
    `;
    alertBox.classList.remove('hidden');

    // Refresh anomalies table so manager can see the blocked incident immediately
    await loadAttendanceAnomalies();
  } finally {
    submitBtn.disabled = false;
    submitBtn.textContent = 'Submit Verified Check-In';
  }
}

/* =====================================================================
   Standard Mark Attendance & Worker Registration
   ===================================================================== */
function openMarkModal() {
  document.getElementById('mark-modal').classList.remove('hidden');
}

function closeMarkModal() {
  document.getElementById('mark-modal').classList.add('hidden');
  document.getElementById('mark-form').reset();
}

async function handleMarkSubmit(e) {
  e.preventDefault();
  const mineId = parseInt(document.getElementById('att-mine').value);
  const workerIdStr = document.getElementById('att-worker').value;
  const workerId = workerIdStr ? parseInt(workerIdStr) : null;
  const date = document.getElementById('att-date').value;
  const shift = document.getElementById('att-shift').value;
  const status = document.getElementById('att-status').value;
  const ot = parseFloat(document.getElementById('att-ot').value) || 0.0;

  try {
    await API.post('/attendance', {
      mine_id: mineId,
      worker_id: workerId,
      record_date: date,
      shift: shift,
      status: status,
      overtime_hours: ot
    });

    showToast('Attendance recorded successfully', 'success');
    closeMarkModal();
    await applyFilters();
  } catch (err) {
    showError(err);
  }
}

function openWorkerModal() {
  document.getElementById('worker-modal').classList.remove('hidden');
}

function closeWorkerModal() {
  document.getElementById('worker-modal').classList.add('hidden');
  document.getElementById('worker-form').reset();
}

async function handleWorkerSubmit(e) {
  e.preventDefault();
  const mineId = parseInt(document.getElementById('w-mine').value);
  const code = document.getElementById('w-code').value.trim();
  const name = document.getElementById('w-name').value.trim();
  const desig = document.getElementById('w-desig').value.trim();
  const contStr = document.getElementById('w-contractor').value;
  const contractorId = contStr ? parseInt(contStr) : null;

  try {
    await API.post('/workers', {
      mine_id: mineId,
      worker_code: code,
      full_name: name,
      designation: desig,
      contractor_id: contractorId
    });

    showToast(`Worker ${name} registered successfully`, 'success');
    closeWorkerModal();
    await loadInitialDropdowns();
  } catch (err) {
    showError(err);
  }
}

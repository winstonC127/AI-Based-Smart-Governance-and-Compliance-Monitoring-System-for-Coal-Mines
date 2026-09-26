/**
 * corrective-actions.js — Corrective actions module logic.
 * Handles task execution, geotagged evidence resolution, verification review, and escalation level tracking.
 */
let allActions = [];
let myTasksOnly = false;

document.addEventListener('DOMContentLoaded', async () => {
  AUTH.guardPage();
  AUTH.renderShell('corrective-actions.html');

  const user = AUTH.getUser();
  const isSupervisor = ['MINE_MANAGER', 'SAFETY_OFFICER', 'SUPER_ADMIN'].includes(user.role_key);

  if (isSupervisor) {
    document.getElementById('run-escalation-btn').classList.remove('hidden');
  }

  // Bind UI Events
  document.getElementById('run-escalation-btn').addEventListener('click', runEscalationCheck);
  document.getElementById('filter-my-tasks').addEventListener('click', toggleMyTasks);
  document.getElementById('filter-status').addEventListener('change', applyFilters);

  // Resolve Modal Events
  document.getElementById('resolve-close').addEventListener('click', closeResolveModal);
  document.getElementById('resolve-cancel').addEventListener('click', closeResolveModal);
  document.getElementById('resolve-form').addEventListener('submit', handleResolveSubmit);
  document.getElementById('btn-resolve-gps').addEventListener('click', captureResolveGPS);

  const resolveEvidence = document.getElementById('resolve-evidence');
  resolveEvidence.addEventListener('change', handleEvidenceFileChange);
  document.getElementById('resolve-lat').addEventListener('input', checkResolveFormValidity);
  document.getElementById('resolve-lng').addEventListener('input', checkResolveFormValidity);

  // Verify Modal Events
  document.getElementById('verify-close').addEventListener('click', closeVerifyModal);
  document.getElementById('verify-cancel').addEventListener('click', closeVerifyModal);
  document.getElementById('verify-form').addEventListener('submit', handleVerifySubmit);

  await loadActions();
});

async function loadActions() {
  const tbody = document.getElementById('actions-table-body');
  tbody.innerHTML = `<tr><td colspan="8" class="state-panel">Loading corrective actions...</td></tr>`;

  try {
    allActions = await API.get('/corrective-actions');
    applyFilters();
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="8" class="state-panel error">${err.message}</td></tr>`;
  }
}

function toggleMyTasks() {
  myTasksOnly = !myTasksOnly;
  const btn = document.getElementById('filter-my-tasks');
  if (myTasksOnly) {
    btn.textContent = 'Show All Tasks';
    btn.classList.add('btn-primary');
    btn.classList.remove('btn-secondary');
  } else {
    btn.textContent = 'Show My Tasks Only';
    btn.classList.add('btn-secondary');
    btn.classList.remove('btn-primary');
  }
  applyFilters();
}

function applyFilters() {
  const status = document.getElementById('filter-status').value;
  const user = AUTH.getUser();

  const filtered = allActions.filter(ca => {
    if (status && ca.status !== status) return false;
    if (myTasksOnly && ca.assigned_to !== user.id) return false;
    return true;
  });

  renderActionsTable(filtered);
}

function renderActionsTable(actions) {
  const tbody = document.getElementById('actions-table-body');
  if (actions.length === 0) {
    tbody.innerHTML = `<tr><td colspan="8" class="state-panel">No corrective actions found.</td></tr>`;
    return;
  }

  const currentUser = AUTH.getUser();
  const isSupervisor = ['MINE_MANAGER', 'SAFETY_OFFICER', 'SUPER_ADMIN'].includes(currentUser.role_key);

  tbody.innerHTML = actions.map(ca => {
    let statusClass = 'inactive';
    if (ca.status === 'CLOSED' || ca.status === 'VERIFIED') statusClass = 'active';
    else if (ca.status === 'ASSIGNED') statusClass = 'info';
    else if (ca.status === 'SUBMITTED') statusClass = 'warning';
    else if (ca.status === 'OVERDUE') statusClass = 'danger';

    let escLabel = '&mdash;';
    if (ca.escalation_level > 0) {
      let escRole = 'Safety Officer';
      if (ca.escalation_level === 2) escRole = 'Mine Manager';
      if (ca.escalation_level === 3) escRole = 'Corporate Management';
      escLabel = `<span class="badge badge-danger">L${ca.escalation_level}: ${escRole}</span>`;
    }

    let actionButtonHtml = '';
    if ((ca.status === 'ASSIGNED' || ca.status === 'OVERDUE') && (ca.assigned_to === currentUser.id || currentUser.role_key === 'SUPER_ADMIN')) {
      actionButtonHtml = `<button class="btn btn-primary btn-sm" onclick="openResolveModal(${ca.id})">Resolve with Proof</button>`;
    } else if (ca.status === 'SUBMITTED' && isSupervisor) {
      actionButtonHtml = `<button class="btn btn-warning btn-sm" onclick="openVerifyModal(${ca.id})">Verify Work</button>`;
    } else {
      actionButtonHtml = `<span class="hint">${ca.status === 'CLOSED' ? 'Closed' : 'Waiting...'}</span>`;
    }

    return `
      <tr>
        <td>
          <span class="mono"><strong>${ca.violation_code}</strong></span><br/>
          <span class="hint" style="font-size:11.5px;">${ca.severity} Severity</span>
        </td>
        <td>${ca.mine_name}</td>
        <td style="max-width:280px; font-size:12.5px; line-height:1.4;">
          <strong>Req:</strong> ${ca.action_description}<br/>
          <span class="hint" style="font-size:11.5px; display:block; margin-top:2px;">Vio: ${ca.violation_desc}</span>
        </td>
        <td>${ca.assigned_to_name}</td>
        <td>${ca.deadline}</td>
        <td><span class="badge badge-${statusClass}">${ca.status}</span></td>
        <td>${escLabel}</td>
        <td>${actionButtonHtml}</td>
      </tr>
    `;
  }).join('');
}

function openResolveModal(id) {
  const ca = allActions.find(a => a.id === id);
  if (!ca) return;

  document.getElementById('resolve-action-id').value = ca.id;
  document.getElementById('resolve-vio-code').textContent = ca.violation_code;
  document.getElementById('resolve-vio-desc').textContent = `${ca.mine_name} — ${ca.action_description}`;
  
  // Reset form fields
  document.getElementById('resolve-evidence').value = '';
  document.getElementById('resolve-preview-container').classList.add('hidden');
  document.getElementById('resolve-preview-img').src = '';
  document.getElementById('resolve-lat').value = '';
  document.getElementById('resolve-lng').value = '';
  document.getElementById('resolve-notes').value = '';
  document.getElementById('resolve-submit').disabled = true;

  document.getElementById('resolve-modal').classList.remove('hidden');

  // Trigger auto-GPS immediately
  captureResolveGPS();
}

function closeResolveModal() {
  document.getElementById('resolve-modal').classList.add('hidden');
}

function handleEvidenceFileChange(e) {
  const file = e.target.files[0];
  const previewContainer = document.getElementById('resolve-preview-container');
  const previewImg = document.getElementById('resolve-preview-img');

  if (file) {
    const reader = new FileReader();
    reader.onload = (ev) => {
      previewImg.src = ev.target.result;
      previewContainer.classList.remove('hidden');
    };
    reader.readAsDataURL(file);
  } else {
    previewContainer.classList.add('hidden');
    previewImg.src = '';
  }
  checkResolveFormValidity();
}

function captureResolveGPS() {
  const latInput = document.getElementById('resolve-lat');
  const lngInput = document.getElementById('resolve-lng');

  latInput.value = '';
  lngInput.value = '';
  latInput.placeholder = 'Locating...';
  lngInput.placeholder = 'Locating...';

  if (!navigator.geolocation) {
    showToast('Geolocation is not supported by your device.', 'error');
    latInput.placeholder = '';
    lngInput.placeholder = '';
    return;
  }

  navigator.geolocation.getCurrentPosition(
    (pos) => {
      latInput.value = pos.coords.latitude.toFixed(6);
      lngInput.value = pos.coords.longitude.toFixed(6);
      latInput.placeholder = '';
      lngInput.placeholder = '';
      showToast('GPS coordinates acquired.', 'success');
      checkResolveFormValidity();
    },
    (err) => {
      showToast('Unable to auto-detect GPS. Fallback coordinates set.', 'warning');
      latInput.value = '22.359500'; // Default Gevra Mine lat
      lngInput.value = '82.689200'; // Default Gevra Mine lng
      latInput.placeholder = '';
      lngInput.placeholder = '';
      checkResolveFormValidity();
    },
    { enableHighAccuracy: true, timeout: 6000 }
  );
}

function checkResolveFormValidity() {
  const fileField = document.getElementById('resolve-evidence');
  const lat = document.getElementById('resolve-lat').value.trim();
  const lng = document.getElementById('resolve-lng').value.trim();
  const submitBtn = document.getElementById('resolve-submit');

  const hasFile = fileField.files && fileField.files.length > 0;
  const hasGPS = lat !== '' && lng !== '' && !isNaN(parseFloat(lat)) && !isNaN(parseFloat(lng));

  submitBtn.disabled = !(hasFile && hasGPS);
}

async function handleResolveSubmit(e) {
  e.preventDefault();
  const submitBtn = document.getElementById('resolve-submit');
  submitBtn.disabled = true;
  submitBtn.textContent = 'Submitting Resolution...';

  const actionId = document.getElementById('resolve-action-id').value;
  const fileField = document.getElementById('resolve-evidence');
  const lat = document.getElementById('resolve-lat').value.trim();
  const lng = document.getElementById('resolve-lng').value.trim();
  const notes = document.getElementById('resolve-notes').value.trim();

  const formData = new FormData();
  if (fileField.files[0]) {
    formData.append('evidence', fileField.files[0]);
  }
  formData.append('gps_latitude', lat);
  formData.append('gps_longitude', lng);
  formData.append('resolution_notes', notes);

  const token = localStorage.getItem('cg_token');
  const API_BASE = window.APP_CONFIG?.API_BASE_URL || 'http://localhost:8080/api';

  try {
    const res = await fetch(`${API_BASE}/corrective-actions/${actionId}/resolve`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`
      },
      body: formData
    });
    const json = await res.json();

    if (!json.success) {
      throw new Error(json.message || 'Failed to submit resolution evidence');
    }

    if (json.data && json.data.off_site_anomaly) {
      showToast(`Submitted with proof. Notice: Geofence perimeter exceeded (${Math.round(json.data.distance_to_mine_m)}m away).`, 'warning');
    } else {
      showToast('Corrective action resolved and submitted for supervisor verification!', 'success');
    }

    closeResolveModal();
    await loadActions();
  } catch (err) {
    showError(err);
  } finally {
    submitBtn.disabled = false;
    submitBtn.textContent = 'Submit Resolution';
  }
}

function openVerifyModal(id) {
  const ca = allActions.find(a => a.id === id);
  if (!ca) return;

  document.getElementById('verify-action-id').value = id;
  document.getElementById('verify-remarks').value = '';
  document.getElementById('verify-decision').value = 'APPROVE';

  // Populate evidence info
  const evidenceContainer = document.getElementById('verify-evidence-container');
  let photoHtml = '';
  if (ca.evidence_photo_path) {
    const API_BASE = (window.APP_CONFIG?.API_BASE_URL || 'http://localhost:8080/api').replace('/api', '');
    const relativePath = ca.evidence_photo_path.replace('./', '');
    photoHtml = `
      <div style="margin-top: 8px;">
        <strong style="font-size:12.5px;">Resolution Photo Evidence:</strong><br/>
        <img src="${API_BASE}/${relativePath}" alt="Resolution Evidence" style="max-width: 100%; max-height: 220px; border-radius: 6px; border: 1px solid var(--color-border); margin-top: 4px;" />
      </div>`;
  } else {
    photoHtml = `<div class="hint" style="margin-top: 6px;">No photo evidence attached.</div>`;
  }

  let gpsHtml = '&mdash;';
  if (ca.resolution_gps_latitude && ca.resolution_gps_longitude) {
    gpsHtml = `<span class="mono">${ca.resolution_gps_latitude.toFixed(6)}, ${ca.resolution_gps_longitude.toFixed(6)}</span>`;
  }

  evidenceContainer.innerHTML = `
    <div style="font-size: 13.5px; margin-bottom: 6px;">
      <strong>Violation:</strong> <span class="mono">${ca.violation_code}</span> &middot; <strong>Mine:</strong> ${ca.mine_name}
    </div>
    <div style="font-size: 12.5px; color: var(--color-ink-muted); margin-bottom: 8px;">
      <strong>Action Required:</strong> ${ca.action_description}
    </div>
    <div style="font-size: 12.5px; margin-bottom: 4px;">
      <strong>Assigned Worker:</strong> ${ca.assigned_to_name} &middot; <strong>Submitted:</strong> ${ca.submitted_at ? new Date(ca.submitted_at).toLocaleString() : 'N/A'}
    </div>
    <div style="font-size: 12.5px; margin-bottom: 4px;">
      <strong>Resolution GPS:</strong> ${gpsHtml}
    </div>
    <div style="font-size: 12.5px; margin-bottom: 8px;">
      <strong>Remediation Notes:</strong> ${ca.resolution_notes || '<span class="hint">No notes provided</span>'}
    </div>
    ${photoHtml}
  `;

  document.getElementById('verify-modal').classList.remove('hidden');
}

function closeVerifyModal() {
  document.getElementById('verify-modal').classList.add('hidden');
}

async function handleVerifySubmit(e) {
  e.preventDefault();
  const saveBtn = document.getElementById('verify-save');
  saveBtn.disabled = true;
  saveBtn.textContent = 'Submitting...';

  const actionId = document.getElementById('verify-action-id').value;
  const remarks = document.getElementById('verify-remarks').value.trim();
  const decision = document.getElementById('verify-decision').value;

  const payload = {
    approved: decision === 'APPROVE',
    remarks: remarks
  };

  try {
    await API.put(`/corrective-actions/${actionId}/verify`, payload);
    showToast('Verification outcome submitted successfully', 'success');
    closeVerifyModal();
    await loadActions();
  } catch (err) {
    showError(err);
  } finally {
    saveBtn.disabled = false;
    saveBtn.textContent = 'Submit Verdict';
  }
}

async function runEscalationCheck() {
  const btn = document.getElementById('run-escalation-btn');
  btn.disabled = true;
  btn.textContent = 'Checking...';

  try {
    const data = await API.post('/corrective-actions/check-escalations');
    showToast(data.message || 'Deadline & escalation checks completed', 'success');
    await loadActions();
  } catch (err) {
    showError(err);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Check Deadlines & Escalations';
  }
}


/**
 * violations.js — Violations Management client logic.
 * Supports manual violation reporting and corrective action assignments.
 */
let allViolations = [];
let allMines = [];
let categories = [];
let assignableUsers = [];

document.addEventListener('DOMContentLoaded', async () => {
  AUTH.guardPage();
  AUTH.renderShell('violations.html');

  const user = AUTH.getUser();
  const canReport = ['MINE_MANAGER', 'SAFETY_OFFICER', 'SUPER_ADMIN'].includes(user.role_key);

  if (canReport) {
    document.getElementById('report-violation-btn').classList.remove('hidden');
  }

  // Bind UI Events
  document.getElementById('report-violation-btn').addEventListener('click', () => openReportModal());
  document.getElementById('modal-close').addEventListener('click', closeReportModal);
  document.getElementById('modal-cancel').addEventListener('click', closeReportModal);
  document.getElementById('violation-form').addEventListener('submit', handleReportSubmit);

  document.getElementById('assign-close').addEventListener('click', closeAssignModal);
  document.getElementById('assign-cancel').addEventListener('click', closeAssignModal);
  document.getElementById('assign-form').addEventListener('submit', handleAssignSubmit);

  document.getElementById('filter-mine').addEventListener('change', applyFilters);
  document.getElementById('filter-severity').addEventListener('change', applyFilters);
  document.getElementById('filter-status').addEventListener('change', applyFilters);

  // Load resources
  await loadMines();
  await loadCategories();
  await loadAssignableUsers();
  await loadViolations();
});

async function loadMines() {
  try {
    allMines = await API.get('/mines');
    const filterMine = document.getElementById('filter-mine');
    const selectMine = document.getElementById('vio-mine');

    const options = allMines.map(m => `<option value="${m.id}">${m.mine_name} (${m.mine_code})</option>`).join('');
    filterMine.innerHTML = `<option value="">All Mines</option>${options}`;
    selectMine.innerHTML = `<option value="" disabled selected>Select Mine...</option>${options}`;
  } catch (err) {
    showError(err);
  }
}

async function loadCategories() {
  try {
    categories = await API.get('/compliance/categories');
    const selectCat = document.getElementById('vio-category');
    selectCat.innerHTML = `<option value="" disabled selected>Select Category...</option>` + 
      categories.map(c => `<option value="${c.id}">${c.name}</option>`).join('');
  } catch (err) {
    showError(err);
  }
}

async function loadAssignableUsers() {
  try {
    // Fetch users (workers / supervisors)
    assignableUsers = await API.get('/users');
    const selectUser = document.getElementById('assign-user');
    
    // Allow assignment to MINE_MANAGER, SAFETY_OFFICER, or INSPECTOR for simple hackathon simulation
    selectUser.innerHTML = `<option value="" disabled selected>Select Assignee...</option>` +
      assignableUsers.map(u => `<option value="${u.id}">${u.full_name} (${u.role_name})</option>`).join('');
  } catch (err) {
    showError(err);
  }
}

async function loadViolations() {
  const tbody = document.getElementById('violations-table-body');
  tbody.innerHTML = `<tr><td colspan="10" class="state-panel">Loading violations...</td></tr>`;

  try {
    allViolations = await API.get('/violations');
    renderViolationsTable(allViolations);
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="10" class="state-panel error">${err.message}</td></tr>`;
  }
}

function applyFilters() {
  const mine = document.getElementById('filter-mine').value;
  const severity = document.getElementById('filter-severity').value;
  const status = document.getElementById('filter-status').value;

  const filtered = allViolations.filter(v => {
    if (mine && String(v.mine_id) !== mine) return false;
    if (severity && v.severity !== severity) return false;
    if (status && v.status !== status) return false;
    return true;
  });

  renderViolationsTable(filtered);
}

function renderViolationsTable(violations) {
  const tbody = document.getElementById('violations-table-body');
  if (violations.length === 0) {
    tbody.innerHTML = `<tr><td colspan="11" class="state-panel">No violations found.</td></tr>`;
    return;
  }

  const user = AUTH.getUser();
  const canAssign = ['MINE_MANAGER', 'SAFETY_OFFICER', 'SUPER_ADMIN'].includes(user.role_key);

  tbody.innerHTML = violations.map(v => {
    let severityClass = 'badge-inactive';
    if (v.severity === 'CRITICAL') severityClass = 'badge-danger';
    else if (v.severity === 'HIGH') severityClass = 'badge-danger'; // or badge-orange
    else if (v.severity === 'MEDIUM') severityClass = 'badge-warning';

    let statusClass = 'inactive';
    if (v.status === 'CLOSED' || v.status === 'VERIFIED') statusClass = 'active';
    else if (v.status === 'OPEN' || v.status === 'OVERDUE') statusClass = 'danger';
    else if (v.status === 'IN_PROGRESS' || v.status === 'RESOLVED') statusClass = 'warning';

    let actionsHtml = '';
    if (v.status === 'OPEN' && canAssign) {
      actionsHtml = `
        <div style="display:flex; gap:6px; justify-content:center; align-items:center;">
          <button class="btn btn-primary btn-sm" onclick="openAssignModal(${v.id}, '${v.violation_code}')">Assign Action</button>
          <button class="btn btn-secondary btn-sm" onclick="dismissViolation(${v.id}, '${v.violation_code}')" title="Dismiss invalid / mistaken violation">Dismiss</button>
        </div>`;
    } else if ((v.status === 'IN_PROGRESS' || v.status === 'OVERDUE') && canAssign) {
      actionsHtml = `<button class="btn btn-secondary btn-sm" onclick="openAssignModal(${v.id}, '${v.violation_code}')">Reassign</button>`;
    } else {
      actionsHtml = `<span class="hint">${v.status === 'CLOSED' || v.status === 'VERIFIED' ? 'Resolved' : 'No action required'}</span>`;
    }

    const formattedDeadline = v.deadline ? v.deadline.substring(0, 10) : '&mdash;';

    return `
      <tr>
        <td class="mono" style="white-space: nowrap;"><strong>${v.violation_code}</strong></td>
        <td style="white-space: nowrap;">${v.mine_name}</td>
        <td style="white-space: nowrap;">${v.category_name}</td>
        <td style="min-width: 220px; font-size: 13px; line-height: 1.4;">${v.description}</td>
        <td style="white-space: nowrap;"><span class="badge ${severityClass}">${v.severity}</span></td>
        <td style="white-space: nowrap;">${renderSLACell(v)}</td>
        <td style="white-space: nowrap;">${v.reported_by_name}</td>
        <td style="white-space: nowrap;">${v.responsible_name || '<span class="hint">Unassigned</span>'}</td>
        <td style="white-space: nowrap; font-family: var(--font-mono);">${formattedDeadline}</td>
        <td style="white-space: nowrap;"><span class="badge badge-${statusClass}">${v.status}</span></td>
        <td style="white-space: nowrap; text-align: center;">${actionsHtml}</td>
      </tr>
    `;
  }).join('');

  startSLACountdownInterval();
}

function renderSLACell(item) {
  const level = item.escalation_level || 1;
  const slaHours = item.sla_hours || 48;
  const createdAt = item.created_at ? new Date(item.created_at).getTime() : Date.now();
  const targetTime = createdAt + slaHours * 3600 * 1000;
  
  let levelBadge = '';
  if (level === 3) {
    levelBadge = `<span class="badge sla-badge-l3" title="Regulatory Breach Alert: Level 3 DGMS Escalation">⚠️ L3 DGMS Alert</span>`;
  } else if (level === 2) {
    levelBadge = `<span class="badge sla-badge-l2" title="Executive Breach Alert: Level 2 HQ Escalation">🔺 L2 HQ Alert</span>`;
  } else {
    levelBadge = `<span class="badge sla-badge-l1" title="Level 1 Site SLA Window: ${slaHours} Hours">L1 Site (${slaHours}h)</span>`;
  }

  const isResolved = ['CLOSED', 'VERIFIED', 'RESOLVED'].includes(item.status);
  
  return `
    <div class="sla-timer-cell">
      <div>${levelBadge}</div>
      <div class="sla-countdown-timer" data-target-time="${targetTime}" data-status="${item.status}">
        ${computeCountdownText(targetTime, isResolved)}
      </div>
    </div>
  `;
}

function computeCountdownText(targetTime, isResolved) {
  if (isResolved) {
    return `<span class="sla-countdown active" style="color:var(--color-low);">✓ SLA Met</span>`;
  }
  const now = Date.now();
  const diff = targetTime - now;

  if (diff <= 0) {
    const elapsedSecs = Math.floor(Math.abs(diff) / 1000);
    const elapsedHours = Math.floor(elapsedSecs / 3600);
    const elapsedMins = Math.floor((elapsedSecs % 3600) / 60);
    return `<span class="sla-countdown breached">⚠️ Breached (+${elapsedHours}h ${elapsedMins}m)</span>`;
  }

  const totalSecs = Math.floor(diff / 1000);
  const hours = Math.floor(totalSecs / 3600);
  const mins = Math.floor((totalSecs % 3600) / 60);
  const secs = totalSecs % 60;

  const isUrgent = hours < 2;
  const cls = isUrgent ? 'urgent' : 'active';
  const icon = isUrgent ? '⏳' : '⏱️';

  return `<span class="sla-countdown ${cls}">${icon} ${hours}h ${mins}m ${secs}s left</span>`;
}

let timerInterval = null;
function startSLACountdownInterval() {
  if (timerInterval) clearInterval(timerInterval);
  timerInterval = setInterval(() => {
    document.querySelectorAll('.sla-countdown-timer').forEach(el => {
      const targetTime = parseInt(el.getAttribute('data-target-time'), 10);
      const status = el.getAttribute('data-status');
      const isResolved = ['CLOSED', 'VERIFIED', 'RESOLVED'].includes(status);
      el.innerHTML = computeCountdownText(targetTime, isResolved);
    });
  }, 1000);
}

function openReportModal() {
  const form = document.getElementById('violation-form');
  form.reset();
  document.getElementById('violation-modal').classList.remove('hidden');
}

function closeReportModal() {
  document.getElementById('violation-modal').classList.add('hidden');
}

async function handleReportSubmit(e) {
  e.preventDefault();
  const saveBtn = document.getElementById('modal-save');
  saveBtn.disabled = true;
  saveBtn.textContent = 'Reporting...';

  const payload = {
    mine_id: parseInt(document.getElementById('vio-mine').value, 10),
    category_id: parseInt(document.getElementById('vio-category').value, 10),
    description: document.getElementById('vio-description').value.trim(),
    severity: document.getElementById('vio-severity').value,
    deadline: document.getElementById('vio-deadline').value
  };

  try {
    await API.post('/violations', payload);
    showToast('Violation logged successfully', 'success');
    closeReportModal();
    await loadViolations();
  } catch (err) {
    showError(err);
  } finally {
    saveBtn.disabled = false;
    saveBtn.textContent = 'Report Violation';
  }
}

function openAssignModal(id, code) {
  document.getElementById('assign-violation-id').value = id;
  document.getElementById('assign-violation-code').value = code;
  document.getElementById('assign-desc').value = '';
  
  // Pre-fill deadline to +3 days from now
  const defaultDeadline = new Date();
  defaultDeadline.setDate(defaultDeadline.getDate() + 3);
  document.getElementById('assign-deadline').value = defaultDeadline.toISOString().split('T')[0];

  document.getElementById('assign-modal').classList.remove('hidden');
}

function closeAssignModal() {
  document.getElementById('assign-modal').classList.add('hidden');
}

async function handleAssignSubmit(e) {
  e.preventDefault();
  const saveBtn = document.getElementById('assign-save');
  saveBtn.disabled = true;
  saveBtn.textContent = 'Assigning...';

  const violationId = parseInt(document.getElementById('assign-violation-id').value, 10);
  const payload = {
    assigned_to: parseInt(document.getElementById('assign-user').value, 10),
    action_description: document.getElementById('assign-desc').value.trim(),
    deadline: document.getElementById('assign-deadline').value
  };

  try {
    await API.put(`/violations/${violationId}/assign`, payload);
    showToast('Violation assigned and corrective action created successfully', 'success');
    closeAssignModal();
    await loadViolations();
  } catch (err) {
    showError(err);
  } finally {
    saveBtn.disabled = false;
    saveBtn.textContent = 'Assign Action';
  }
}

async function dismissViolation(id, code) {
  const reason = prompt(`Reason for dismissing/closing Violation ${code} (e.g. false alarm / duplicate / mistaken report):`);
  if (!reason || reason.trim().length < 10) {
    if (reason !== null) {
      showToast('A reason of at least 10 characters is required to dismiss', 'warning');
    }
    return;
  }
  try {
    await API.put(`/violations/${id}/dismiss`, { reason: reason.trim() });
    showToast(`Violation ${code} dismissed successfully`, 'success');
    await loadViolations();
  } catch (err) {
    showError(err);
  }
}

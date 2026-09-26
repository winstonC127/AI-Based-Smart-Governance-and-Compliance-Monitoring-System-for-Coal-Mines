/**
 * incidents.js — Incidents tracking & offline reporting logic.
 */
let allIncidents = [];
let allMines = [];

document.addEventListener('DOMContentLoaded', async () => {
  AUTH.guardPage();
  AUTH.renderShell('incidents.html');

  // Set default datetime to now
  const now = new Date();
  now.setMinutes(now.getMinutes() - now.getTimezoneOffset());
  document.getElementById('inc-date').value = now.toISOString().slice(0, 16);

  document.getElementById('filter-mine').addEventListener('change', loadIncidents);
  document.getElementById('filter-severity').addEventListener('change', loadIncidents);
  document.getElementById('filter-status').addEventListener('change', loadIncidents);

  document.getElementById('btn-report-incident').addEventListener('click', openIncidentModal);
  document.getElementById('incident-modal-close').addEventListener('click', closeIncidentModal);
  document.getElementById('incident-modal-cancel').addEventListener('click', closeIncidentModal);
  document.getElementById('incident-form').addEventListener('submit', handleIncidentSubmit);

  await loadMines();
  await loadIncidents();
});

async function loadMines() {
  try {
    allMines = await API.get('/mines');
    const filterMine = document.getElementById('filter-mine');
    const incMine = document.getElementById('inc-mine');

    const opts = allMines.map(m => `<option value="${m.id}">${m.mine_name} (${m.mine_code})</option>`).join('');
    filterMine.innerHTML = `<option value="">All Mines</option>${opts}`;
    incMine.innerHTML = opts;
  } catch (err) {
    showError(err);
  }
}

async function loadIncidents() {
  const tbody = document.getElementById('incidents-table-body');
  tbody.innerHTML = `<tr><td colspan="9" class="state-panel">Loading incidents...</td></tr>`;

  try {
    const mineId = document.getElementById('filter-mine').value;
    const severity = document.getElementById('filter-severity').value;
    const status = document.getElementById('filter-status').value;

    let url = `/incidents?`;
    if (mineId) url += `mine_id=${mineId}&`;
    if (severity) url += `severity=${severity}&`;
    if (status) url += `status=${status}&`;

    allIncidents = await API.get(url);
    updateKPIs(allIncidents);
    renderIncidentsTable(allIncidents);
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="9" class="state-panel error">${err.message}</td></tr>`;
  }
}

function updateKPIs(items) {
  document.getElementById('kpi-total-inc').textContent = items.length;
  document.getElementById('kpi-critical-inc').textContent = items.filter(i => i.severity === 'CRITICAL').length;
  document.getElementById('kpi-open-inc').textContent = items.filter(i => i.status === 'OPEN' || i.status === 'UNDER_REVIEW').length;
  document.getElementById('kpi-closed-inc').textContent = items.filter(i => i.status === 'CLOSED').length;
}

const INCIDENT_TYPE_MAP = {
  'EMERGENCY_SOS': { label: 'Emergency SOS', icon: 'siren', tagClass: '' },
  'FIRE': { label: 'Fire Outbreak', icon: 'flame', tagClass: '' },
  'GAS_LEAK': { label: 'Gas Leak', icon: 'wind', tagClass: '' },
  'ROOF_FALL': { label: 'Roof / Strata Fall', icon: 'layers', tagClass: '' },
  'FLOODING': { label: 'Flooding / Inrush', icon: 'droplet', tagClass: '' },
  'EQUIPMENT_FAILURE': { label: 'Equipment Failure', icon: 'wrench', tagClass: 'tag-warning' },
  'WORKER_TRAPPED': { label: 'Worker Trapped', icon: 'alert', tagClass: '' },
  'EXPLOSION': { label: 'Explosion / Blast', icon: 'sparkles', tagClass: '' },
  'MEDICAL_EMERGENCY': { label: 'Medical Emergency', icon: 'cross', tagClass: '' },
  'OTHER': { label: 'Other Emergency', icon: 'warning', tagClass: 'tag-info' },
  'PIT_SLOPE_FAILURE': { label: 'Pit Slope Failure', icon: 'layers', tagClass: '' },
  'Safety Hazard / Near Miss': { label: 'Safety Hazard / Near Miss', icon: 'warning', tagClass: 'tag-warning' },
  'Machinery / HEMM Breakdown': { label: 'Machinery / HEMM Breakdown', icon: 'wrench', tagClass: 'tag-warning' },
  'Gas / Environmental Release': { label: 'Gas / Environmental Release', icon: 'wind', tagClass: 'tag-warning' },
  'Fire / Spontaneous Combustion': { label: 'Fire / Spontaneous Combustion', icon: 'flame', tagClass: '' },
  'Slope / Strata Instability': { label: 'Slope / Strata Instability', icon: 'layers', tagClass: '' },
  'Medical / Injury': { label: 'Medical / Injury', icon: 'cross', tagClass: '' },
  'Other Incident': { label: 'Other Incident', icon: 'incidents', tagClass: 'tag-info' }
};

function formatIncidentType(rawType) {
  const meta = INCIDENT_TYPE_MAP[rawType] || { label: rawType || 'Incident', icon: 'incidents', tagClass: 'tag-info' };
  const iconSvg = window.ICONS ? ICONS.get(meta.icon) : '';
  return `
    <div class="incident-type-tag ${meta.tagClass || ''}">
      <span class="type-icon">${iconSvg}</span>
      <span>${meta.label}</span>
    </div>
  `;
}

function renderIncidentsTable(items) {
  const tbody = document.getElementById('incidents-table-body');
  const user = AUTH.getUser();
  const canManage = ['MINE_MANAGER', 'SAFETY_OFFICER', 'SUPER_ADMIN'].includes(user.role_key);

  if (!items || items.length === 0) {
    tbody.innerHTML = `<tr><td colspan="9" class="state-panel">No incidents match current filters.</td></tr>`;
    return;
  }

  tbody.innerHTML = items.map(i => {
    let sevBadge = 'badge-info';
    if (i.severity === 'CRITICAL') sevBadge = 'badge-critical';
    else if (i.severity === 'HIGH') sevBadge = 'badge-warning';

    let statusBadge = 'badge-draft';
    if (i.status === 'UNDER_REVIEW') statusBadge = 'badge-warning';
    else if (i.status === 'CLOSED') statusBadge = 'badge-active';

    const incDate = i.incident_date ? new Date(i.incident_date).toLocaleString('en-IN') : '-';

    let actions = '-';
    if (canManage && i.status !== 'CLOSED') {
      actions = `
        <div style="display:flex; gap:6px;">
          ${i.status === 'OPEN' ? `<button class="btn btn-secondary btn-sm" onclick="updateIncidentStatus(${i.id}, 'UNDER_REVIEW')">Review</button>` : ''}
          <button class="btn btn-primary btn-sm" onclick="updateIncidentStatus(${i.id}, 'CLOSED')">Close</button>
        </div>
      `;
    }

    const hasMeshRelay = i.description && i.description.includes('Mesh Relay');
    const meshBadge = hasMeshRelay ? `
      <div style="margin-top:6px;">
        <button class="btn btn-secondary btn-sm" onclick="viewRelayPath(${i.id})" style="font-size:11px; padding:2px 8px; gap:4px; border-color:#38bdf8; color:#0284c7;">
          <span>⚡</span> Mesh Hops
        </button>
      </div>` : '';

    return `
      <tr>
        <td class="mono font-semibold">#${i.id}</td>
        <td><strong>${i.mine_name}</strong></td>
        <td>${formatIncidentType(i.incident_type)}</td>
        <td><span class="badge ${sevBadge}">${i.severity}</span></td>
        <td>
          <div style="max-width:320px; line-height:1.4;">${i.description}</div>
          ${meshBadge}
        </td>
        <td>${i.reported_by_name}</td>
        <td class="mono">${incDate}</td>
        <td><span class="badge ${statusBadge}">${i.status}</span></td>
        <td>${actions}</td>
      </tr>
    `;
  }).join('');
}

function openIncidentModal() {
  document.getElementById('incident-modal').classList.remove('hidden');
}

function closeIncidentModal() {
  document.getElementById('incident-modal').classList.add('hidden');
  document.getElementById('incident-form').reset();
}

async function handleIncidentSubmit(e) {
  e.preventDefault();
  const mineId = parseInt(document.getElementById('inc-mine').value);
  const incType = document.getElementById('inc-type').value;
  const severity = document.getElementById('inc-sev').value;
  const incDate = document.getElementById('inc-date').value.replace('T', ' ') + ':00';
  const desc = document.getElementById('inc-desc').value.trim();

  const payload = {
    mine_id: mineId,
    incident_type: incType,
    severity: severity,
    incident_date: incDate,
    description: desc
  };

  // Offline Check
  if (!navigator.onLine) {
    if (window.OfflineStore) {
      await window.OfflineStore.save('incidents', payload);
      showToast('Offline Mode: Incident report saved to local queue. Will sync when online.', 'warning');
      closeIncidentModal();
      return;
    }
  }

  try {
    await API.post('/incidents', payload);
    showToast('Incident report filed successfully and notifications sent.', 'success');
    closeIncidentModal();
    await loadIncidents();
  } catch (err) {
    if (window.OfflineStore && !navigator.onLine) {
      await window.OfflineStore.save('incidents', payload);
      showToast('Incident saved locally in offline queue.', 'warning');
      closeIncidentModal();
    } else {
      showError(err);
    }
  }
}

window.updateIncidentStatus = async function(id, newStatus) {
  if (!confirm(`Update incident #${id} to status ${newStatus}?`)) return;
  try {
    await API.put(`/incidents/${id}/status`, { status: newStatus });
    showToast(`Incident marked as ${newStatus}`, 'success');
    await loadIncidents();
  } catch (err) {
    showError(err);
  }
};

window.viewRelayPath = async function(incidentId) {
  let modal = document.getElementById('relay-path-modal');
  if (!modal) {
    modal = document.createElement('div');
    modal.id = 'relay-path-modal';
    modal.className = 'modal-backdrop hidden';
    modal.innerHTML = `
      <div class="modal" style="max-width:540px;">
        <div class="modal-header">
          <h3 style="display:flex; align-items:center; gap:8px;">
            <span>⚡</span> Underground Mesh Telemetry Path
          </h3>
          <button type="button" class="modal-close" id="relay-modal-close">&times;</button>
        </div>
        <div class="modal-body" id="relay-modal-body">
          <div class="state-panel">Loading telemetry hop logs...</div>
        </div>
        <div class="modal-footer">
          <button type="button" class="btn btn-secondary" id="relay-modal-dismiss">Close</button>
        </div>
      </div>
    `;
    document.body.appendChild(modal);

    document.getElementById('relay-modal-close').addEventListener('click', () => modal.classList.add('hidden'));
    document.getElementById('relay-modal-dismiss').addEventListener('click', () => modal.classList.add('hidden'));
    modal.addEventListener('click', (e) => { if (e.target === modal) modal.classList.add('hidden'); });
  }

  modal.classList.remove('hidden');
  const bodyEl = document.getElementById('relay-modal-body');
  bodyEl.innerHTML = `<div class="state-panel">Resolving hop logs for incident #${incidentId}...</div>`;

  try {
    const data = await API.get(`/incidents/${incidentId}/relay-path`);
    const hops = data.hops || [];
    if (hops.length === 0) {
      bodyEl.innerHTML = `<div class="state-panel">No underground mesh relay hops recorded for incident #${incidentId}.</div>`;
      return;
    }

    const hopRows = hops.map((h, idx) => {
      const isGateway = idx === hops.length - 1;
      return `
        <div class="mesh-hop-node visible" style="margin-bottom:8px;">
          <div class="mesh-node-info">
            <div class="mesh-node-dot ${isGateway ? 'gateway' : ''}"></div>
            <div>
              <span class="mesh-node-name">Hop ${h.hop_number}: ${h.node_name}</span>
              <span style="font-size:10px; color:#94a3b8; margin-left:6px;">(Seq: ${h.hop_sequence})</span>
            </div>
          </div>
          <div class="mesh-node-metrics">
            <span class="mesh-signal-tag">📶 ${Math.round(h.signal_strength_pct)}%</span>
            <span class="mesh-latency-tag">⚡ ${h.latency_ms}ms</span>
          </div>
        </div>
      `;
    }).join('');

    bodyEl.innerHTML = `
      <div class="mesh-telemetry-panel" style="margin-top:0;">
        <div class="mesh-telemetry-header">
          <div class="mesh-telemetry-title">
            <span>📡</span> Incident #${incidentId} Multi-Hop Chain
          </div>
          <div class="mesh-telemetry-badge">${hops.length} Hops &middot; ${data.total_latency_ms}ms Total Latency</div>
        </div>
        <div class="mesh-hop-list">
          ${hopRows}
        </div>
        <div class="mesh-gateway-alert visible" style="margin-top:12px;">
          <span>🚨</span> Surface Gateway verified — Alert dispatched to responders
        </div>
      </div>
    `;
  } catch (err) {
    bodyEl.innerHTML = `<div class="state-panel error">${err.message || 'Failed to fetch relay path'}</div>`;
  }
};

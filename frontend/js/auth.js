/**
 * auth.js — login flow, session storage, route guarding, and the shared
 * sidebar/topbar shell that every authenticated page renders.
 */
const AUTH = (() => {
  function saveSession(token, user) {
    localStorage.setItem('cg_token', token);
    localStorage.setItem('cg_user', JSON.stringify(user));
  }

  function getUser() {
    const raw = localStorage.getItem('cg_user');
    return raw ? JSON.parse(raw) : null;
  }

  function isLoggedIn() {
    return !!localStorage.getItem('cg_token');
  }

  function clearSession() {
    localStorage.removeItem('cg_token');
    localStorage.removeItem('cg_user');
  }

  // Call at the top of every protected page.
  function guardPage() {
    if (!isLoggedIn()) {
      location.href = 'login.html';
      return;
    }
    const user = getUser();
    if (!user) return;

    // Detect the current page name
    const pageName = location.pathname.split('/').pop() || 'index.html';
    const item = NAV_ITEMS.find(n => n.href === pageName);
    if (item && item.roles !== 'ALL') {
      const hasRole = item.roles.includes(user.role_key) || user.role_key === 'SUPER_ADMIN';
      if (!hasRole) {
        location.href = 'dashboard.html';
      }
    }
  }

  async function logout() {
    try {
      await API.post('/auth/logout');
    } catch (e) {
      // Even if the network call fails, clear the local session.
    }
    clearSession();
    location.href = 'login.html';
  }

  // Sidebar items, annotated with the roles allowed to see them.
  // SUPER_ADMIN implicitly sees everything.
  const NAV_ITEMS = [
    { href: 'dashboard.html',   label: 'Dashboard',          iconKey: 'dashboard', roles: 'ALL' },
    { href: 'mines.html',       label: 'Mines',               iconKey: 'mines', roles: 'ALL' },
    { href: 'inspections.html', label: 'Inspections',         iconKey: 'inspections', roles: ['SUPER_ADMIN', 'INSPECTOR', 'SAFETY_OFFICER', 'MINE_MANAGER'] },
    { href: 'violations.html',  label: 'Violations',          iconKey: 'violations', roles: ['SUPER_ADMIN', 'SAFETY_OFFICER', 'MINE_MANAGER', 'REGULATORY_OFFICER', 'CORPORATE_MANAGER'] },
    { href: 'corrective-actions.html', label: 'Corrective Actions', iconKey: 'corrective_actions', roles: ['SUPER_ADMIN', 'SAFETY_OFFICER', 'MINE_MANAGER', 'REGULATORY_OFFICER'] },
    { href: 'incidents.html',   label: 'Incidents & SOS',     iconKey: 'incidents', roles: 'ALL' },
    { href: 'attendance.html',  label: 'Attendance',          iconKey: 'attendance', roles: 'ALL' },
    { href: 'grievances.html',  label: 'Grievances',          iconKey: 'grievances', roles: 'ALL' },
    { href: 'contractors.html', label: 'Contractors',         iconKey: 'contractors', roles: 'ALL' },
    { href: 'environmental.html', label: 'Environmental',     iconKey: 'environmental', roles: 'ALL' },
    { href: 'production.html',  label: 'Production & HEMM',   iconKey: 'production', roles: 'ALL' },
    { href: 'compliance.html',  label: 'Compliance Rules',    iconKey: 'compliance', roles: 'ALL' },
    { href: 'documents.html',   label: 'Documents & OCR',     iconKey: 'documents', roles: 'ALL' },
    { href: 'analytics.html',   label: 'AI Analytics',        iconKey: 'analytics', roles: 'ALL' },
    { href: 'reports.html',     label: 'Reports',              iconKey: 'reports', roles: ['SUPER_ADMIN', 'MINE_MANAGER', 'REGULATORY_OFFICER', 'CORPORATE_MANAGER'] },
    { href: 'notifications.html', label: 'Notifications',      iconKey: 'notifications', roles: 'ALL' },
    { href: 'simulation-control.html', label: 'Simulation Control', iconKey: 'simulation', roles: ['SUPER_ADMIN'] },
    { href: 'audit-logs.html',  label: 'Audit Trail',         iconKey: 'audit', roles: ['SUPER_ADMIN', 'REGULATORY_OFFICER', 'CORPORATE_MANAGER'] },
    { href: 'users.html',       label: 'Users',                 iconKey: 'users', roles: ['SUPER_ADMIN'] }
  ];

  function renderShell(activePage) {
    const user = getUser();
    if (!user) return;

    const sidebarEl = document.getElementById('app-sidebar');
    const topbarUserEl = document.getElementById('topbar-user-info');

    if (sidebarEl) {
      const links = NAV_ITEMS
        .filter((item) => item.roles === 'ALL' || item.roles.includes(user.role_key) || user.role_key === 'SUPER_ADMIN')
        .map((item) => {
          const activeClass = item.href === activePage ? 'active' : '';
          const iconSvg = window.ICONS ? ICONS.get(item.iconKey) : '';
          return `<a href="${item.href}" class="${activeClass}"><span class="icon">${iconSvg}</span>${item.label}</a>`;
        })
        .join('');

      sidebarEl.innerHTML = `
        <div class="sidebar-brand">
          <div class="org">Coal India Limited &middot; Ministry of Coal</div>
          <div class="title">Smart Governance &amp; Compliance Platform</div>
        </div>
        <div class="strata-band"></div>
        <nav class="sidebar-nav">${links}</nav>
        <div class="sidebar-footer">SIH26024 &middot; Demo Build</div>
      `;
    }

    if (topbarUserEl) {
      topbarUserEl.innerHTML = `
        <span>${user.full_name}</span>
        <span class="role-tag">${user.role_name}</span>
        <button id="logout-btn">Log out</button>
      `;
      document.getElementById('logout-btn').addEventListener('click', logout);
    }

    EMERGENCY.mount();
  }

  return { saveSession, getUser, isLoggedIn, clearSession, guardPage, logout, renderShell };
})();

/**
 * EMERGENCY — the global Emergency SOS button + modal.
 * Mounted once per page (by AUTH.renderShell) so it's available everywhere
 * an authenticated user can be, regardless of which page they're on.
 */
const EMERGENCY = (() => {
  let mounted = false;
  let mines = [];
  let sending = false;
  let selectedPreset = null;

  function updateSubmitState() {
    const submitBtn = document.getElementById('sos-submit');
    if (!submitBtn) return;
    const desc = (document.getElementById('sos-description')?.value || '').trim();
    const hasValidInput = !!selectedPreset || desc.length > 0;
    submitBtn.disabled = !hasValidInput || sending;
  }

  function mount() {
    if (mounted || document.getElementById('sos-fab')) return;
    mounted = true;

    const sirenIcon = window.ICONS ? ICONS.get('siren') : '';
    const flameIcon = window.ICONS ? ICONS.get('flame') : '';
    const windIcon = window.ICONS ? ICONS.get('wind') : '';
    const layersIcon = window.ICONS ? ICONS.get('layers') : '';
    const dropletIcon = window.ICONS ? ICONS.get('droplet') : '';
    const wrenchIcon = window.ICONS ? ICONS.get('wrench') : '';
    const alertIcon = window.ICONS ? ICONS.get('alert') : '';
    const sparklesIcon = window.ICONS ? ICONS.get('sparkles') : '';
    const crossIcon = window.ICONS ? ICONS.get('cross') : '';
    const warningIcon = window.ICONS ? ICONS.get('warning') : '';

    const fab = document.createElement('button');
    fab.id = 'sos-fab';
    fab.className = 'sos-fab';
    fab.type = 'button';
    fab.innerHTML = `<span class="sos-icon" style="display:inline-flex; align-items:center;">${sirenIcon}</span> Emergency SOS`;
    fab.addEventListener('click', openModal);
    document.body.appendChild(fab);

    const backdrop = document.createElement('div');
    backdrop.id = 'sos-modal';
    backdrop.className = 'modal-backdrop hidden';
    backdrop.innerHTML = `
      <div class="modal sos-modal">
        <div class="modal-header">
          <h3 style="display:flex; align-items:center; gap:8px;">${sirenIcon} Emergency SOS</h3>
          <button type="button" class="modal-close" id="sos-close">&times;</button>
        </div>
        <form id="sos-form">
          ${renderModalFormHTML()}
        </form>
      </div>
    `;
    document.body.appendChild(backdrop);

    document.getElementById('sos-close').addEventListener('click', closeModal);
    backdrop.addEventListener('click', (e) => {
      if (e.target === backdrop) closeModal();
    });
    document.getElementById('sos-form').addEventListener('submit', handleSubmit);
    resetModalToForm();
  }
  function renderModalFormHTML() {
    const sirenIcon = window.ICONS ? ICONS.get('siren') : '';
    const flameIcon = window.ICONS ? ICONS.get('flame') : '';
    const windIcon = window.ICONS ? ICONS.get('wind') : '';
    const layersIcon = window.ICONS ? ICONS.get('layers') : '';
    const dropletIcon = window.ICONS ? ICONS.get('droplet') : '';
    const wrenchIcon = window.ICONS ? ICONS.get('wrench') : '';
    const alertIcon = window.ICONS ? ICONS.get('alert') : '';
    const sparklesIcon = window.ICONS ? ICONS.get('sparkles') : '';
    const crossIcon = window.ICONS ? ICONS.get('cross') : '';
    const warningIcon = window.ICONS ? ICONS.get('warning') : '';

    return `
      <div class="modal-body" id="sos-modal-body-container">
        <p class="sos-warning">This immediately logs a CRITICAL incident and alerts mine
          management, safety officers, and regulatory oversight in real time.
          Only use this for a genuine emergency.</p>
        <div class="form-group">
          <label for="sos-mine">Mine Site</label>
          <select id="sos-mine" required></select>
        </div>
        <div class="sos-presets-section">
          <label class="sos-presets-label">Emergency Type (Quick Select)</label>
          <div class="sos-presets-grid" id="sos-presets-grid">
            <button type="button" class="sos-preset-btn" data-type="FIRE">
              <span class="preset-icon">${flameIcon}</span> Fire
            </button>
            <button type="button" class="sos-preset-btn" data-type="GAS_LEAK">
              <span class="preset-icon">${windIcon}</span> Gas Leak
            </button>
            <button type="button" class="sos-preset-btn" data-type="ROOF_FALL">
              <span class="preset-icon">${layersIcon}</span> Roof Fall
            </button>
            <button type="button" class="sos-preset-btn" data-type="FLOODING">
              <span class="preset-icon">${dropletIcon}</span> Flooding
            </button>
            <button type="button" class="sos-preset-btn" data-type="EQUIPMENT_FAILURE">
              <span class="preset-icon">${wrenchIcon}</span> Equip. Failure
            </button>
            <button type="button" class="sos-preset-btn" data-type="WORKER_TRAPPED">
              <span class="preset-icon">${alertIcon}</span> Worker Trapped
            </button>
            <button type="button" class="sos-preset-btn" data-type="EXPLOSION">
              <span class="preset-icon">${sparklesIcon}</span> Explosion
            </button>
            <button type="button" class="sos-preset-btn" data-type="MEDICAL_EMERGENCY">
              <span class="preset-icon">${crossIcon}</span> Medical
            </button>
            <button type="button" class="sos-preset-btn" data-type="OTHER">
              <span class="preset-icon">${warningIcon}</span> Other
            </button>
          </div>
        </div>
        <div class="form-group">
          <label for="sos-description">What's happening? (optional detail)</label>
          <textarea id="sos-description" rows="3" placeholder="e.g. Near sector 4, two workers trapped, evacuation started..."></textarea>
        </div>
      </div>
      <div class="modal-footer" id="sos-modal-footer-container">
        <button type="button" class="btn btn-secondary" id="sos-cancel">Cancel</button>
        <button type="submit" class="btn btn-danger" id="sos-submit" disabled>Send SOS Alert</button>
      </div>
    `;
  }

  function resetModalToForm() {
    const form = document.getElementById('sos-form');
    if (!form) return;
    form.innerHTML = renderModalFormHTML();

    document.getElementById('sos-cancel').addEventListener('click', closeModal);
    const descEl = document.getElementById('sos-description');
    descEl.addEventListener('input', updateSubmitState);

    document.querySelectorAll('.sos-preset-btn').forEach((btn) => {
      btn.addEventListener('click', () => {
        const type = btn.dataset.type;
        if (selectedPreset === type) {
          selectedPreset = null;
          btn.classList.remove('active');
        } else {
          document.querySelectorAll('.sos-preset-btn').forEach((b) => b.classList.remove('active'));
          btn.classList.add('active');
          selectedPreset = type;

          if (type === 'OTHER') {
            descEl.classList.add('sos-textarea-prompt');
            descEl.focus();
            setTimeout(() => {
              descEl.classList.remove('sos-textarea-prompt');
            }, 1200);
          }
        }
        updateSubmitState();
      });
    });

    populateMineSelect();
  }

  function populateMineSelect() {
    const select = document.getElementById('sos-mine');
    if (!select) return;
    if (mines.length > 0) {
      select.innerHTML = mines
        .map((m) => `<option value="${m.id}">${m.mine_name}</option>`)
        .join('');
    } else {
      select.innerHTML = '<option>Loading mines...</option>';
    }
  }

  async function openModal() {
    selectedPreset = null;
    resetModalToForm();
    updateSubmitState();

    document.getElementById('sos-modal').classList.remove('hidden');
    if (mines.length === 0) {
      try {
        mines = await API.get('/mines');
        populateMineSelect();
      } catch (err) {
        const select = document.getElementById('sos-mine');
        if (select) select.innerHTML = '<option value="">Could not load mines</option>';
        showError(err);
      }
    }
  }

  function closeModal() {
    document.getElementById('sos-modal').classList.add('hidden');
    selectedPreset = null;
    resetModalToForm();
    updateSubmitState();
  }

  async function animateMeshRelay(hops, totalLatency) {
    const bodyContainer = document.getElementById('sos-modal-body-container');
    const footerContainer = document.getElementById('sos-modal-footer-container');
    if (!bodyContainer) return;

    if (footerContainer) footerContainer.innerHTML = '<div style="font-size:12px; color:var(--color-ink-muted);">Mesh multi-hop transmission in progress...</div>';

    const windIcon = window.ICONS ? ICONS.get('wind') : '';
    const sirenIcon = window.ICONS ? ICONS.get('siren') : '';

    bodyContainer.innerHTML = `
      <div class="mesh-telemetry-panel">
        <div class="mesh-telemetry-header">
          <div class="mesh-telemetry-title">
            <span>${windIcon}</span> Underground Mesh Telemetry Simulation
          </div>
          <div class="mesh-telemetry-badge" id="mesh-live-status">TRANSMITTING...</div>
        </div>
        <div class="mesh-hop-list" id="mesh-live-hop-list"></div>
        <div class="mesh-gateway-alert" id="mesh-live-gateway-alert">
          <span>${sirenIcon}</span> Surface Gateway reached — Dashboard alerted (${totalLatency}ms total latency)
        </div>
      </div>
    `;

    const hopList = document.getElementById('mesh-live-hop-list');

    for (let i = 0; i < hops.length; i++) {
      const hop = hops[i];
      const isGateway = (i === hops.length - 1);
      const nodeEl = document.createElement('div');
      nodeEl.className = 'mesh-hop-node';
      nodeEl.innerHTML = `
        <div class="mesh-node-info">
          <div class="mesh-node-dot ${isGateway ? 'gateway' : ''}"></div>
          <div>
            <span class="mesh-node-name">Hop ${hop.hop_number}: ${hop.node_name}</span>
          </div>
        </div>
        <div class="mesh-node-metrics">
          <span class="mesh-signal-tag">📶 ${Math.round(hop.signal_strength_pct)}%</span>
          <span class="mesh-latency-tag">⚡ ${hop.latency_ms}ms</span>
        </div>
      `;
      hopList.appendChild(nodeEl);

      await new Promise((r) => setTimeout(r, 60));
      nodeEl.classList.add('visible', 'active-transmit');
      await new Promise((r) => setTimeout(r, 260));
      nodeEl.classList.remove('active-transmit');
    }

    const gatewayAlert = document.getElementById('mesh-live-gateway-alert');
    const statusBadge = document.getElementById('mesh-live-status');
    if (gatewayAlert) gatewayAlert.classList.add('visible');
    if (statusBadge) {
      statusBadge.textContent = 'DISPATCHED';
      statusBadge.style.color = '#10b981';
      statusBadge.style.background = 'rgba(16,185,129,0.15)';
    }

    await new Promise((r) => setTimeout(r, 900));
  }

  async function handleSubmit(e) {
    e.preventDefault();
    if (sending) return;

    const mineId = document.getElementById('sos-mine')?.value;
    const description = (document.getElementById('sos-description')?.value || '').trim();
    if (!mineId) {
      showToast('Select a mine first', 'error');
      return;
    }

    if (!selectedPreset && !description) {
      showToast('Select an emergency type or provide a description', 'warning');
      return;
    }

    const submitBtn = document.getElementById('sos-submit');
    sending = true;
    if (submitBtn) {
      submitBtn.disabled = true;
      submitBtn.textContent = 'Transmitting Mesh Packet...';
    }

    try {
      const result = await API.post('/incidents/emergency', {
        mine_id: parseInt(mineId, 10),
        incident_type: selectedPreset || 'EMERGENCY_SOS',
        description,
      });

      if (result.relay_hops && result.relay_hops.length > 0) {
        await animateMeshRelay(result.relay_hops, result.total_latency_ms || 0);
      }

      closeModal();
      showToast(`Emergency alert sent — ${result.notified_count} responder(s) notified`, 'success');

      if (window.loadIncidents) {
        window.loadIncidents();
      }
    } catch (err) {
      showError(err);
    } finally {
      sending = false;
      updateSubmitState();
    }
  }

  return { mount };
})();

// ---------------- Login page handler ----------------
document.addEventListener('DOMContentLoaded', () => {
  const loginForm = document.getElementById('login-form');
  if (!loginForm) return;

  // If already logged in, skip straight to the dashboard.
  if (AUTH.isLoggedIn()) {
    location.href = 'dashboard.html';
    return;
  }

  const params = new URLSearchParams(location.search);
  if (params.get('expired') === '1') {
    showToast('Your session expired. Please log in again.', 'error');
  }

  // Clicking a demo account row fills the form for convenience.
  document.querySelectorAll('.demo-row').forEach((row) => {
    row.addEventListener('click', () => {
      document.getElementById('email').value = row.dataset.email;
      document.getElementById('password').value = 'Coal@2026';
    });
  });

  loginForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const email = document.getElementById('email').value.trim();
    const password = document.getElementById('password').value;
    const errorEl = document.getElementById('login-error');
    const submitBtn = document.getElementById('login-submit');

    errorEl.textContent = '';
    submitBtn.disabled = true;
    submitBtn.textContent = 'Signing in...';

    try {
      const data = await API.post('/auth/login', { email, password });
      AUTH.saveSession(data.token, data.user);
      location.href = 'dashboard.html';
    } catch (err) {
      errorEl.textContent = err.message || 'Login failed';
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = 'Sign in';
    }
  });
});

/**
 * contractors.js — Contractor Lifecycle & Compliance client logic.
 */
let allContractors = [];
let allMines = [];

document.addEventListener('DOMContentLoaded', async () => {
  AUTH.guardPage();
  AUTH.renderShell('contractors.html');

  const user = AUTH.getUser();
  const canManage = ['SUPER_ADMIN', 'MINE_MANAGER'].includes(user.role_key);
  if (canManage) {
    const addBtn = document.getElementById('btn-add-contractor');
    if (addBtn) addBtn.classList.remove('hidden');
  }

  document.getElementById('search-contractor').addEventListener('input', applyFilters);
  document.getElementById('filter-status').addEventListener('change', applyFilters);
  const filterMine = document.getElementById('filter-mine');
  if (filterMine) filterMine.addEventListener('change', applyFilters);

  document.getElementById('btn-check-expiries').addEventListener('click', handleCheckExpiries);

  const addBtn = document.getElementById('btn-add-contractor');
  if (addBtn) addBtn.addEventListener('click', () => openContractorModal());
  document.getElementById('contractor-modal-close').addEventListener('click', closeContractorModal);
  document.getElementById('contractor-modal-cancel').addEventListener('click', closeContractorModal);
  document.getElementById('contractor-form').addEventListener('submit', handleContractorSubmit);

  document.getElementById('blacklist-modal-close').addEventListener('click', closeBlacklistModal);
  document.getElementById('blacklist-modal-cancel').addEventListener('click', closeBlacklistModal);
  document.getElementById('blacklist-form').addEventListener('submit', handleBlacklistSubmit);

  await Promise.all([
    loadMines(),
    loadContractors()
  ]);
});

async function loadMines() {
  try {
    allMines = await API.get('/mines').catch(() => []);
    
    // Filter dropdown
    const filterMine = document.getElementById('filter-mine');
    if (filterMine) {
      filterMine.innerHTML = '<option value="">All Mines</option>' +
        allMines.map(m => `<option value="${m.id}">${m.mine_name} (${m.mine_code})</option>`).join('');
    }

    // Modal dropdown
    const cMine = document.getElementById('c-mine');
    if (cMine) {
      cMine.innerHTML = allMines.length ? 
        allMines.map(m => `<option value="${m.id}">${m.mine_name} (${m.mine_code})</option>`).join('') :
        '<option value="">Default Mine</option>';
    }
  } catch (err) {
    console.error('Failed to load mines in contractors:', err);
  }
}

async function loadContractors() {
  const tbody = document.getElementById('contractors-table-body');
  tbody.innerHTML = `<tr><td colspan="10" class="state-panel">Loading contractors...</td></tr>`;

  try {
    allContractors = await API.get('/contractors');
    updateKPIs(allContractors);
    applyFilters();
  } catch (err) {
    tbody.innerHTML = `<tr><td colspan="10" class="state-panel error">${err.message}</td></tr>`;
  }
}

function updateKPIs(items) {
  const now = new Date();
  const thirtyDaysOut = new Date(now.getTime() + (30 * 24 * 60 * 60 * 1000));

  let active = 0;
  let expiring = 0;
  let blacklisted = 0;

  items.forEach(c => {
    if (c.status === 'BLACKLISTED') {
      blacklisted++;
    } else if (c.status === 'ACTIVE') {
      active++;
      if (c.contract_end) {
        const endDate = new Date(c.contract_end);
        if (endDate > now && endDate <= thirtyDaysOut) {
          expiring++;
        }
      }
    }
  });

  document.getElementById('kpi-total-c').textContent = items.length;
  document.getElementById('kpi-active-c').textContent = active;
  document.getElementById('kpi-expiring-c').textContent = expiring;
  document.getElementById('kpi-blacklisted-c').textContent = blacklisted;
}

function applyFilters() {
  const query = document.getElementById('search-contractor').value.toLowerCase().trim();
  const status = document.getElementById('filter-status').value;
  const filterMine = document.getElementById('filter-mine');
  const mineId = filterMine ? filterMine.value : '';

  const filtered = allContractors.filter(c => {
    if (status && c.status !== status) return false;
    if (mineId && String(c.mine_id) !== String(mineId)) return false;
    if (query) {
      const matchName = c.company_name?.toLowerCase().includes(query);
      const matchPerson = c.contact_person?.toLowerCase().includes(query);
      const matchEmail = c.email?.toLowerCase().includes(query);
      const matchType = c.contract_type?.toLowerCase().includes(query);
      const matchMine = c.mine_name?.toLowerCase().includes(query);
      if (!matchName && !matchPerson && !matchEmail && !matchType && !matchMine) return false;
    }
    return true;
  });

  renderContractorsTable(filtered);
}

function renderContractorsTable(items) {
  const tbody = document.getElementById('contractors-table-body');
  const user = AUTH.getUser();
  const canManage = ['SUPER_ADMIN', 'MINE_MANAGER', 'SAFETY_OFFICER'].includes(user.role_key);

  if (!items || items.length === 0) {
    tbody.innerHTML = `<tr><td colspan="10" class="state-panel">No contractors found matching current filters.</td></tr>`;
    return;
  }

  tbody.innerHTML = items.map(c => {
    let badgeClass = 'badge-active';
    if (c.status === 'EXPIRED') badgeClass = 'badge-warning';
    else if (c.status === 'BLACKLISTED') badgeClass = 'badge-critical';
    else if (c.status === 'SUSPENDED') badgeClass = 'badge-critical';

    const period = `${c.contract_start ? c.contract_start.split('T')[0] : '-'} &rarr; ${c.contract_end ? c.contract_end.split('T')[0] : '-'}`;

    let actions = '-';
    if (canManage) {
      actions = `
        <div style="display:flex; gap:6px; flex-wrap:wrap;">
          <button class="btn btn-secondary btn-sm" onclick="openContractorModal(${c.id})">Edit</button>
          ${c.status !== 'BLACKLISTED' ? `<button class="btn btn-danger btn-sm" onclick="openBlacklistModal(${c.id})">Blacklist</button>` : ''}
        </div>
      `;
    }

    return `
      <tr>
        <td class="mono font-semibold" style="color:var(--color-brand);">#${c.id}</td>
        <td>
          <div style="font-weight:600; color:var(--color-ink);">${c.company_name}</div>
          ${c.blacklist_reason ? `<div style="font-size:11px; color:var(--color-critical); margin-top:2px;">Reason: ${c.blacklist_reason}</div>` : ''}
        </td>
        <td>
          <div style="font-weight:500;">${c.mine_name || 'General / All Sites'}</div>
        </td>
        <td>
          <span style="font-size:12.5px; background:var(--color-bg-light); padding:2px 8px; border-radius:4px; border:1px solid var(--color-border);">${c.contract_type || 'General Operations'}</span>
        </td>
        <td>${c.contact_person || '-'}</td>
        <td>
          <div>${c.email || '-'}</div>
          <div class="mono hint" style="font-size:11.5px;">${c.phone || '-'}</div>
        </td>
        <td class="mono" style="font-size:12px;">${period}</td>
        <td>
          <span class="badge badge-info" style="font-size:11px; font-weight:600;">${c.worker_count || 0} deployed</span>
        </td>
        <td><span class="badge ${badgeClass}">${c.status}</span></td>
        <td>${actions}</td>
      </tr>
    `;
  }).join('');
}

window.openContractorModal = function(id) {
  const form = document.getElementById('contractor-form');
  form.reset();

  const cMine = document.getElementById('c-mine');
  const cType = document.getElementById('c-type');
  const cStatus = document.getElementById('c-status');

  if (id) {
    const c = allContractors.find(item => String(item.id) === String(id));
    if (!c) return;
    document.getElementById('contractor-modal-title').textContent = `Edit Contractor Details — ${c.company_name}`;
    document.getElementById('c-id').value = c.id;
    document.getElementById('c-name').value = c.company_name;
    document.getElementById('c-person').value = c.contact_person || '';
    document.getElementById('c-email').value = c.email || '';
    document.getElementById('c-phone').value = c.phone || '';
    document.getElementById('c-start').value = c.contract_start ? c.contract_start.split('T')[0] : '';
    document.getElementById('c-end').value = c.contract_end ? c.contract_end.split('T')[0] : '';
    if (cMine && c.mine_id) cMine.value = c.mine_id;
    if (cType && c.contract_type) cType.value = c.contract_type;
    if (cStatus && c.status) cStatus.value = c.status;
  } else {
    document.getElementById('contractor-modal-title').textContent = 'Register Contractor Agency';
    document.getElementById('c-id').value = '';
    if (cMine && allMines.length > 0) cMine.value = allMines[0].id;
    if (cType) cType.value = 'Overburden Removal';
    if (cStatus) cStatus.value = 'ACTIVE';
  }

  document.getElementById('contractor-modal').classList.remove('hidden');
};

function closeContractorModal() {
  document.getElementById('contractor-modal').classList.add('hidden');
}

async function handleContractorSubmit(e) {
  e.preventDefault();
  const id = document.getElementById('c-id').value;
  const cMine = document.getElementById('c-mine');
  const cType = document.getElementById('c-type');
  const cStatus = document.getElementById('c-status');

  const payload = {
    company_name: document.getElementById('c-name').value.trim(),
    contact_person: document.getElementById('c-person').value.trim(),
    email: document.getElementById('c-email').value.trim(),
    phone: document.getElementById('c-phone').value.trim(),
    contract_start: document.getElementById('c-start').value,
    contract_end: document.getElementById('c-end').value,
    mine_id: cMine && cMine.value ? parseInt(cMine.value, 10) : 0,
    contract_type: cType ? cType.value : 'General Operations',
    status: cStatus ? cStatus.value : 'ACTIVE'
  };

  try {
    if (id) {
      await API.put(`/contractors/${id}`, payload);
      showToast('Contractor updated successfully', 'success');
    } else {
      await API.post('/contractors', payload);
      showToast('Contractor registered successfully', 'success');
    }
    closeContractorModal();
    await loadContractors();
  } catch (err) {
    showError(err);
  }
}

window.openBlacklistModal = function(id) {
  document.getElementById('bl-id').value = id;
  document.getElementById('bl-reason').value = '';
  document.getElementById('blacklist-modal').classList.remove('hidden');
};

function closeBlacklistModal() {
  document.getElementById('blacklist-modal').classList.add('hidden');
}

async function handleBlacklistSubmit(e) {
  e.preventDefault();
  const id = document.getElementById('bl-id').value;
  const reason = document.getElementById('bl-reason').value.trim();

  try {
    await API.put(`/contractors/${id}/blacklist`, { blacklist_reason: reason });
    showToast('Contractor blacklisted and compliance alert dispatched.', 'warning');
    closeBlacklistModal();
    await loadContractors();
  } catch (err) {
    showError(err);
  }
}

async function handleCheckExpiries() {
  const btn = document.getElementById('btn-check-expiries');
  btn.disabled = true;
  btn.textContent = 'Scanning Expiries...';

  try {
    const res = await API.post('/contractors/check-expiries', {});
    showToast(res.message || 'Expiries checked successfully', 'info');
    await loadContractors();
  } catch (err) {
    showError(err);
  } finally {
    const alertSvg = window.ICONS ? ICONS.get('alert') : '';
    btn.innerHTML = `${alertSvg} Scan Expiries &amp; Alert`;
  }
}

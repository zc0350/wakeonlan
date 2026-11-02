'use strict';

// ---------- 通用工具 ----------
const $ = (sel) => document.querySelector(sel);

function toast(msg, type) {
  const el = $('#toast');
  el.textContent = msg;
  el.className = 'toast show' + (type ? ' ' + type : '');
  setTimeout(() => { el.className = 'toast'; }, 2600);
}

async function api(path, options = {}) {
  options.headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
  const res = await fetch(path, options);
  if (res.status === 401) {
    window.location.href = '/login';
    throw new Error('unauthorized');
  }
  let data = null;
  try { data = await res.json(); } catch (e) { /* 可能无响应体 */ }
  if (!res.ok) {
    const msg = (data && data.error) || ('请求失败 (' + res.status + ')');
    throw new Error(msg);
  }
  return data;
}

function openModal(id) { $('#' + id).classList.add('show'); }
function closeModal(id) { $('#' + id).classList.remove('show'); }

// 点击遮罩或带 data-close 的按钮关闭模态框
document.addEventListener('click', (e) => {
  if (e.target.classList && e.target.classList.contains('modal-mask')) {
    e.target.classList.remove('show');
  }
  const closeId = e.target.getAttribute && e.target.getAttribute('data-close');
  if (closeId) closeModal(closeId);
});

// ---------- 初始化 ----------
async function init() {
  try {
    const me = await api('/api/me');
    $('#curUser').textContent = me.username;
  } catch (e) {
    return; // 已被 api() 重定向
  }
  bindTabs();
  bindDeviceEvents();
  bindScanEvents();
  bindAdminEvents();
  bindMyPwdEvents();
  await loadDevices();
  await loadAdmins();
}

function bindTabs() {
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach((t) => t.classList.remove('active'));
      tab.classList.add('active');
      const target = tab.getAttribute('data-tab');
      $('#tab-devices').style.display = target === 'devices' ? '' : 'none';
      $('#tab-admins').style.display = target === 'admins' ? '' : 'none';
    });
  });
}

// ---------- 设备管理 ----------
async function loadDevices() {
  const data = await api('/api/devices');
  const body = $('#deviceBody');
  body.innerHTML = '';
  const devices = data.devices || [];
  $('#devCount').textContent = '共 ' + devices.length + ' 台设备';
  $('#deviceEmpty').style.display = devices.length ? 'none' : 'block';
  devices.forEach((d) => {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td><input type="checkbox" class="dev-check" value="' + d.id + '"></td>' +
      '<td>' + esc(d.name) + '</td>' +
      '<td class="mono">' + esc(d.mac) + '</td>' +
      '<td class="mono">' + esc(d.broadcast) + '</td>' +
      '<td class="notes-cell" title="' + esc(d.notes) + '">' + esc(d.notes || '-') + '</td>' +
      '<td>' +
        '<button class="btn btn-sm btn-primary" data-wake="' + d.id + '">唤醒</button> ' +
        '<button class="btn btn-sm" data-edit="' + d.id + '">编辑</button> ' +
        '<button class="btn btn-sm btn-danger" data-del="' + d.id + '">删除</button>' +
      '</td>';
    body.appendChild(tr);
  });
  bindDeviceRowEvents();
}

function bindDeviceRowEvents() {
  document.querySelectorAll('[data-wake]').forEach((b) => {
    b.addEventListener('click', () => wakeOne(parseInt(b.getAttribute('data-wake'), 10)));
  });
  document.querySelectorAll('[data-edit]').forEach((b) => {
    b.addEventListener('click', () => editDevice(parseInt(b.getAttribute('data-edit'), 10)));
  });
  document.querySelectorAll('[data-del]').forEach((b) => {
    b.addEventListener('click', () => deleteDevice(parseInt(b.getAttribute('data-del'), 10)));
  });
}

function bindDeviceEvents() {
  $('#checkAll').addEventListener('change', (e) => {
    document.querySelectorAll('.dev-check').forEach((c) => { c.checked = e.target.checked; });
  });
  $('#btnRefreshDevices').addEventListener('click', loadDevices);
  $('#btnAddDevice').addEventListener('click', () => {
    $('#deviceModalTitle').textContent = '添加设备';
    $('#deviceId').value = '';
    $('#dName').value = '';
    $('#dMac').value = '';
    $('#dBroadcast').value = '255.255.255.255';
    $('#dNotes').value = '';
    $('#deviceErr').textContent = '';
    openModal('deviceModal');
  });
  $('#btnSaveDevice').addEventListener('click', saveDevice);
  $('#btnWakeSelected').addEventListener('click', wakeSelected);
}

async function saveDevice() {
  const id = $('#deviceId').value;
  const payload = {
    name: $('#dName').value.trim(),
    mac: $('#dMac').value.trim(),
    broadcast: $('#dBroadcast').value.trim(),
    notes: $('#dNotes').value.trim()
  };
  try {
    if (id) {
      await api('/api/devices/' + id, { method: 'PUT', body: JSON.stringify(payload) });
      toast('设备已更新', 'ok');
    } else {
      await api('/api/devices', { method: 'POST', body: JSON.stringify(payload) });
      toast('设备已添加', 'ok');
    }
    closeModal('deviceModal');
    await loadDevices();
  } catch (e) {
    $('#deviceErr').textContent = e.message;
  }
}

async function editDevice(id) {
  const data = await api('/api/devices');
  const d = (data.devices || []).find((x) => x.id === id);
  if (!d) return;
  $('#deviceModalTitle').textContent = '编辑设备';
  $('#deviceId').value = d.id;
  $('#dName').value = d.name;
  $('#dMac').value = d.mac;
  $('#dBroadcast').value = d.broadcast || '255.255.255.255';
  $('#dNotes').value = d.notes || '';
  $('#deviceErr').textContent = '';
  openModal('deviceModal');
}

async function deleteDevice(id) {
  if (!confirm('确定删除该设备？')) return;
  try {
    await api('/api/devices/' + id, { method: 'DELETE' });
    toast('已删除', 'ok');
    await loadDevices();
  } catch (e) {
    toast(e.message, 'err');
  }
}

async function wakeOne(id) {
  try {
    const res = await api('/api/devices/' + id + '/wake', { method: 'POST' });
    if (res.error) toast('唤醒失败：' + res.error, 'err');
    else toast('已向「' + res.name + '」发送唤醒包', 'ok');
  } catch (e) {
    toast(e.message, 'err');
  }
}

async function wakeSelected() {
  const ids = Array.from(document.querySelectorAll('.dev-check:checked')).map((c) => parseInt(c.value, 10));
  if (!ids.length) { toast('请先勾选设备', 'err'); return; }
  try {
    const data = await api('/api/devices/wake', { method: 'POST', body: JSON.stringify({ ids }) });
    showWakeResults(data.results || []);
  } catch (e) {
    toast(e.message, 'err');
  }
}

function showWakeResults(results) {
  const okCount = results.filter((r) => !r.error).length;
  const failCount = results.length - okCount;
  let html = '<div class="result-list">';
  html += '<div>本次共 ' + results.length + ' 台，成功 ' + okCount + ' 台，失败 ' + failCount + ' 台。</div>';
  results.forEach((r) => {
    if (r.error) {
      html += '<div class="row fail">✗ ' + esc(r.name) + ' (' + esc(r.mac) + ')：' + esc(r.error) + '</div>';
    } else {
      html += '<div class="row ok">✓ ' + esc(r.name) + ' (' + esc(r.mac) + ')</div>';
    }
  });
  html += '</div>';
  toast('批量唤醒完成：成功 ' + okCount + ' / 失败 ' + failCount, failCount ? 'err' : 'ok');
  // 在设备页顶部临时展示明细
  const empty = $('#deviceEmpty');
  const detail = document.getElementById('wakeDetail');
  if (detail) detail.remove();
  const div = document.createElement('div');
  div.id = 'wakeDetail';
  div.className = 'panel';
  div.style.marginTop = '14px';
  div.innerHTML = html;
  $('#tab-devices').appendChild(div);
  setTimeout(() => { if (div) div.remove(); }, 8000);
}

// ---------- 局域网扫描 ----------
function bindScanEvents() {
  $('#btnScanLAN').addEventListener('click', openScanModal);
  $('#btnDoScan').addEventListener('click', runScan);
  $('#scanCidr').addEventListener('keydown', (e) => { if (e.key === 'Enter') runScan(); });
  $('#scanCheckAll').addEventListener('change', (e) => {
    document.querySelectorAll('.scan-check').forEach((c) => { c.checked = e.target.checked; });
    updateScanCount();
  });
  $('#btnAddScanned').addEventListener('click', addScanned);
}

async function openScanModal() {
  $('#scanCidr').value = '';
  $('#scanBody').innerHTML = '';
  $('#scanEmpty').style.display = 'none';
  $('#scanErr').textContent = '';
  $('#scanStatus').textContent = '';
  $('#btnAddScanned').disabled = true;
  openModal('scanModal');
  await runScan(); // 打开弹窗即自动扫描
}

async function runScan() {
  const cidr = $('#scanCidr').value.trim();
  $('#scanStatus').textContent = '正在扫描（约需 1-3 秒）…';
  $('#scanErr').textContent = '';
  $('#scanBody').innerHTML = '';
  $('#scanEmpty').style.display = 'none';
  $('#btnAddScanned').disabled = true;
  try {
    const q = cidr ? ('?cidr=' + encodeURIComponent(cidr)) : '';
    const data = await api('/api/lan/scan' + q);
    const hosts = data.hosts || [];
    $('#scanStatus').textContent =
      '扫描完成：发现 ' + hosts.length + ' 台设备（网段 ' + (data.cidrs || []).join('、') + '）';
    if (!hosts.length) {
      $('#scanEmpty').style.display = 'block';
      return;
    }
    const body = $('#scanBody');
    hosts.forEach((h) => {
      const tr = document.createElement('tr');
      tr.innerHTML =
        '<td><input type="checkbox" class="scan-check" data-mac="' + esc(h.mac) + '"' +
        (h.exists ? '' : ' checked') + '></td>' +
        '<td class="mono">' + esc(h.ip) + '</td>' +
        '<td class="mono">' + esc(h.mac) + '</td>' +
        '<td class="notes-cell" title="' + esc(h.hostname || '') + '">' + esc(h.hostname || '-') + '</td>';
      body.appendChild(tr);
    });
    document.querySelectorAll('.scan-check').forEach((c) => {
      c.addEventListener('change', updateScanCount);
    });
    updateScanCount();
  } catch (e) {
    $('#scanStatus').textContent = '';
    $('#scanErr').textContent = e.message;
  }
}

function updateScanCount() {
  const n = document.querySelectorAll('.scan-check:checked').length;
  const btn = $('#btnAddScanned');
  btn.disabled = n === 0;
  btn.textContent = n ? '添加选中设备（' + n + '）' : '添加选中设备';
}

async function addScanned() {
  const items = Array.from(document.querySelectorAll('.scan-check:checked')).map((c) => {
    const tds = c.closest('tr').querySelectorAll('td');
    const ip = tds[1].textContent.trim();
    const hostname = tds[3].textContent.trim();
    return {
      name: hostname && hostname !== '-' ? hostname : ip,
      mac: c.getAttribute('data-mac'),
      broadcast: '255.255.255.255',
      notes: '扫描添加自 ' + ip
    };
  });
  if (!items.length) return;
  try {
    const data = await api('/api/devices/batch', { method: 'POST', body: JSON.stringify({ items }) });
    toast('已添加 ' + data.created + ' 台设备，跳过 ' + data.skipped + ' 台（已存在）', 'ok');
    closeModal('scanModal');
    await loadDevices();
  } catch (e) {
    $('#scanErr').textContent = e.message;
  }
}

// ---------- 管理员管理 ----------
async function loadAdmins() {
  const data = await api('/api/admins');
  const body = $('#adminBody');
  body.innerHTML = '';
  const admins = data.admins || [];
  $('#adminEmpty').style.display = admins.length ? 'none' : 'block';
  admins.forEach((a) => {
    const tr = document.createElement('tr');
    const actions =
      '<button class="btn btn-sm" data-reset="' + a.id + '">重置密码</button> ' +
      '<button class="btn btn-sm btn-danger" data-admindel="' + a.id + '">删除</button>';
    tr.innerHTML =
      '<td>' + esc(a.username) + '</td>' +
      '<td class="mono">' + esc(a.created_at || '') + '</td>' +
      '<td>' + actions + '</td>';
    body.appendChild(tr);
  });
  document.querySelectorAll('[data-reset]').forEach((b) => {
    const id = parseInt(b.getAttribute('data-reset'), 10);
    const row = b.closest('tr');
    const name = row ? row.querySelector('td').textContent : '';
    b.addEventListener('click', () => openReset(id, name));
  });
  document.querySelectorAll('[data-admindel]').forEach((b) => {
    b.addEventListener('click', () => deleteAdmin(parseInt(b.getAttribute('data-admindel'), 10)));
  });
}

function bindAdminEvents() {
  $('#btnRefreshAdmins').addEventListener('click', loadAdmins);
  $('#btnAddAdmin').addEventListener('click', () => {
    $('#aName').value = '';
    $('#aPassword').value = '';
    $('#adminErr').textContent = '';
    openModal('adminModal');
  });
  $('#btnSaveAdmin').addEventListener('click', async () => {
    const payload = { username: $('#aName').value.trim(), password: $('#aPassword').value };
    try {
      await api('/api/admins', { method: 'POST', body: JSON.stringify(payload) });
      closeModal('adminModal');
      toast('管理员已创建', 'ok');
      await loadAdmins();
    } catch (e) {
      $('#adminErr').textContent = e.message;
    }
  });
  $('#btnSaveReset').addEventListener('click', async () => {
    const id = $('#resetId').value;
    const payload = { password: $('#resetPassword').value };
    try {
      await api('/api/admins/' + id + '/password', { method: 'PUT', body: JSON.stringify(payload) });
      closeModal('resetModal');
      toast('密码已重置', 'ok');
    } catch (e) {
      $('#resetErr').textContent = e.message;
    }
  });
}

function openReset(id, name) {
  $('#resetId').value = id;
  $('#resetName').textContent = name || ('#' + id);
  $('#resetPassword').value = '';
  $('#resetErr').textContent = '';
  openModal('resetModal');
}

async function deleteAdmin(id) {
  if (!confirm('确定删除该管理员？')) return;
  try {
    await api('/api/admins/' + id, { method: 'DELETE' });
    toast('已删除管理员', 'ok');
    await loadAdmins();
  } catch (e) {
    toast(e.message, 'err');
  }
}

// ---------- 修改自己的密码 ----------
function bindMyPwdEvents() {
  $('#btnMyPwd').addEventListener('click', () => {
    $('#oldPwd').value = '';
    $('#newPwd').value = '';
    $('#myPwdErr').textContent = '';
    openModal('myPwdModal');
  });
  $('#btnSaveMyPwd').addEventListener('click', async () => {
    const payload = { old_password: $('#oldPwd').value, new_password: $('#newPwd').value };
    try {
      await api('/api/admins/me/password', { method: 'PUT', body: JSON.stringify(payload) });
      closeModal('myPwdModal');
      toast('密码已修改，请重新登录', 'ok');
      setTimeout(() => { window.location.href = '/login'; }, 1200);
    } catch (e) {
      $('#myPwdErr').textContent = e.message;
    }
  });
  $('#btnLogout').addEventListener('click', async () => {
    await api('/api/logout', { method: 'POST' });
    window.location.href = '/login';
  });
}

// ---------- HTML 转义 ----------
function esc(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

init();

// Flowzen shared helpers: API client, layout, modals, toasts, formatting.
// Change the backend address here (or run: localStorage.fz_api = "https://your-api").
// When the backend serves these pages (localhost:8080 or your live domain) it uses the same address;
// when opened from Live Server or another local port it talks to http://localhost:8080.
const API_URL = localStorage.getItem('fz_api')
  || (location.port === '8080' || !['localhost', '127.0.0.1', ''].includes(location.hostname) ? location.origin : 'http://localhost:8080');

const $ = (s, el = document) => el.querySelector(s);
const qs = (k) => new URLSearchParams(location.search).get(k);
const esc = (v) => String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const label = (s) => esc(String(s || '').replace(/_/g, ' '));
const STATUSES = ['pending', 'in_progress', 'review', 'blocked', 'completed'];
const PRIORITIES = ['low', 'medium', 'high', 'urgent'];
const PROJECT_STATUSES = ['planning', 'active', 'on_hold', 'completed', 'cancelled'];
let ME = null, MY_ADMIN = null;

async function api(path, opts = {}) {
  const headers = {};
  const token = localStorage.getItem('fz_token');
  if (token) headers.Authorization = 'Bearer ' + token;
  let body = opts.body;
  if (body && !(body instanceof FormData)) { headers['Content-Type'] = 'application/json'; body = JSON.stringify(body); }
  let res;
  try {
    res = await fetch(API_URL + '/api' + path, { method: opts.method || (body ? 'POST' : 'GET'), headers, body });
  } catch { throw new Error('Cannot reach the Flowzen server. Check that the backend is running.'); }
  if (res.status === 401 && token && !path.startsWith('/auth/')) {
    localStorage.removeItem('fz_token');
    location.href = 'index.html?expired=1';
    throw new Error('Session expired');
  }
  if (opts.raw && res.ok) return res;
  const data = await res.json().catch(() => ({}));
  if (!res.ok || data.success === false) throw new Error(data.error || 'Request failed (' + res.status + ')');
  return data.data !== undefined ? data.data : data;
}

// ---------- UI helpers ----------
function toast(msg, type = 'success') {
  let box = $('#toasts');
  if (!box) { box = document.createElement('div'); box.id = 'toasts'; document.body.appendChild(box); }
  const t = document.createElement('div');
  t.className = 'toast ' + type; t.setAttribute('role', 'status'); t.textContent = msg;
  box.appendChild(t); setTimeout(() => t.remove(), 4000);
}
const fail = (e) => toast(e.message || String(e), 'error');

function fmtDate(d) {
  if (!d) return '—';
  const dt = d.length === 10 ? new Date(d + 'T00:00:00') : new Date(d);
  return dt.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}
function fmtTime(d) { return d ? new Date(d).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '—'; }
function fmtSize(n) { return n > 1048576 ? (n / 1048576).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB'; }
const badge = (v, cls) => v ? `<span class="badge b-${esc(cls || v)}">${label(v)}</span>` : '';
const progressBar = (p) => `<div class="row"><div class="progress" style="flex:1"><div style="width:${+p || 0}%"></div></div><span class="small">${+p || 0}%</span></div>`;
function avatar(u, cls = '') {
  if (!u) return '';
  if (u.avatar) return `<img class="avatar ${cls}" src="${esc(API_URL + u.avatar)}" alt="${esc(u.name)}">`;
  return `<span class="avatar ${cls}" title="${esc(u.name)}">${esc((u.name || '?').split(' ').map((x) => x[0]).join('').slice(0, 2).toUpperCase())}</span>`;
}
const loading = (el) => { el.innerHTML = '<div class="loading">Loading…</div>'; };
const empty = (text) => `<div class="empty">${esc(text)}</div>`;
const options = (list, selected, labels) => list.map((v, i) => `<option value="${esc(v)}" ${[].concat(selected).includes(v) ? 'selected' : ''}>${labels ? esc(labels[i]) : label(v)}</option>`).join('');
const userOptions = (users, selected = []) => users.map((u) => `<option value="${u.id}" ${[].concat(selected).includes(u.id) ? 'selected' : ''}>${esc(u.name)} (${esc(u.email)})</option>`).join('');

// formObj turns a form into an object; multi-selects become arrays, checkboxes booleans.
function formObj(form) {
  const o = {};
  for (const el of form.elements) {
    if (!el.name) continue;
    if (el.type === 'checkbox') o[el.name] = el.checked;
    else if (el.multiple) o[el.name] = [...el.selectedOptions].map((x) => x.value);
    else if (el.type !== 'file') o[el.name] = el.value.trim();
  }
  return o;
}

// openModal shows a form dialog; onSubmit(data, form) may be async; return false to keep it open.
function openModal(title, html, onSubmit, submitText = 'Save') {
  const ov = document.createElement('div');
  ov.className = 'overlay';
  ov.innerHTML = `<form class="modal" role="dialog" aria-modal="true" aria-label="${esc(title)}" novalidate><h2>${esc(title)}</h2>
    <div class="error-box hidden"></div>${html}<div class="row end"><button type="button" class="btn light" data-close>${onSubmit ? 'Cancel' : 'Close'}</button>
    ${onSubmit ? `<button class="btn" type="submit">${esc(submitText)}</button>` : ''}</div></form>`;
  document.body.appendChild(ov);
  const form = $('form', ov), err = $('.error-box', ov), close = () => ov.remove();
  ov.addEventListener('click', (e) => { if (e.target === ov || e.target.hasAttribute('data-close')) close(); });
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    if (!form.checkValidity()) { err.textContent = 'Please fill in all required fields correctly.'; err.classList.remove('hidden'); form.reportValidity(); return; }
    const btn = $('button[type=submit]', form);
    btn.disabled = true; err.classList.add('hidden');
    try { if ((await onSubmit(formObj(form), form)) !== false) close(); }
    catch (ex) { err.textContent = ex.message; err.classList.remove('hidden'); }
    btn.disabled = false;
  });
  const first = $('input,select,textarea', form); if (first) first.focus();
  return form;
}

function confirmBox(text, okText = 'Delete') {
  return new Promise((resolve) => {
    const ov = document.createElement('div');
    ov.className = 'overlay';
    ov.innerHTML = `<div class="modal" role="alertdialog" aria-modal="true"><h2>Please confirm</h2><p>${esc(text)}</p>
      <div class="row end"><button class="btn light" data-v="0">Cancel</button><button class="btn danger" data-v="1">${esc(okText)}</button></div></div>`;
    document.body.appendChild(ov);
    ov.addEventListener('click', (e) => {
      const v = e.target.getAttribute('data-v');
      if (v !== null || e.target === ov) { ov.remove(); resolve(v === '1'); }
    });
    $('[data-v="1"]', ov).focus();
  });
}

function pager(el, res, go) {
  if (!res || res.pages <= 1) { el.innerHTML = res && res.total ? `<div class="pager small muted">${res.total} record(s)</div>` : ''; return; }
  el.innerHTML = `<div class="pager"><span class="small muted">${res.total} records · page ${res.page} of ${res.pages}</span>
    <button class="btn light sm" ${res.page <= 1 ? 'disabled' : ''} data-p="${res.page - 1}">‹ Prev</button>
    <button class="btn light sm" ${res.page >= res.pages ? 'disabled' : ''} data-p="${res.page + 1}">Next ›</button></div>`;
  el.querySelectorAll('[data-p]').forEach((b) => b.onclick = () => go(+b.dataset.p));
}

function activityList(items) {
  if (!items || !items.length) return empty('No activity yet');
  return items.map((a) => `<div class="list-item"><div><b>${esc(a.actorName)}</b> ${esc(a.action)}${a.details ? ` <span class="muted">— ${esc(a.details)}</span>` : ''}</div>
    <span class="small muted" style="white-space:nowrap">${fmtTime(a.createdAt)}</span></div>`).join('');
}

const isApproved = () => ME && (['admin', 'sysadmin'].includes(ME.role) || !!ME.adminId);
const isAdmin = () => ME && ['admin', 'sysadmin'].includes(ME.role);

// ---------- page shell ----------
async function layout(active, { approval = true, roles } = {}) {
  if (!localStorage.getItem('fz_token')) { location.href = 'index.html'; return false; }
  const page = $('#page');
  page.innerHTML = '<div class="loading">Loading…</div>' + page.innerHTML;
  let me;
  try { me = await api('/me'); } catch (e) { fail(e); return false; }
  ME = me.user; MY_ADMIN = me.admin;
  $('.loading', page)?.remove();
  if (approval && !isApproved()) { location.href = 'requests.html'; return false; }
  if (roles && !roles.includes(ME.role)) { location.href = 'dashboard.html'; return false; }

  const link = (href, text, key) => `<a href="${href}" class="${active === key ? 'active' : ''}">${text}</a>`;
  let nav = link('dashboard.html', '🏠 Dashboard', 'dashboard');
  if (isApproved()) nav += '<div class="sect">Work</div>' + link('workspaces.html', '🗂️ Workspaces', 'workspaces') + link('projects.html', '📁 Projects', 'projects')
    + link('tasks.html', '✅ Tasks', 'tasks') + link('reports.html', '📊 Reports', 'reports');
  nav += '<div class="sect">Team</div>' + link('requests.html', isAdmin() ? '📨 Access Requests' : '📨 My Administrator', 'requests');
  if (ME.role === 'admin') nav += link('team.html', '👥 My Team', 'team');
  if (ME.role === 'sysadmin') nav += '<div class="sect">System</div>' + link('admin.html', '🛡️ System Admin', 'admin');
  nav += '<div class="sect">Account</div>' + link('notifications.html', '🔔 Notifications', 'notifications') + link('profile.html', '👤 Profile', 'profile');

  const shell = document.createElement('div');
  shell.className = 'app';
  shell.innerHTML = `<nav class="sidebar" aria-label="Main navigation"><div class="logo"><span>F</span>Flowzen</div>${nav}</nav>
    <div class="main"><header class="topbar"><button class="menu-btn" aria-label="Toggle menu">☰</button><div class="muted hide-sm">${esc(MY_ADMIN ? 'Team: ' + MY_ADMIN.name : label(ME.role))}</div>
      <div class="actions"><a class="bell" href="notifications.html" aria-label="Notifications">🔔<span class="count hidden" id="notif-count"></span></a>
      <div class="profile-menu"><button aria-haspopup="true" aria-label="Profile menu">${avatar(ME)}<span class="hide-sm">${esc(ME.name)}</span> ▾</button>
      <div class="dropdown"><div class="small muted" style="padding:6px 10px">${esc(ME.email)}<br>${label(ME.role)}</div>
      <a href="profile.html">My profile</a><a href="notifications.html">Notifications</a><button id="logout">Log out</button></div></div></div></header>
      <main class="content"></main></div>`;
  document.body.prepend(shell);
  $('.content', shell).appendChild(page);
  $('.menu-btn', shell).onclick = () => $('.sidebar').classList.toggle('open');
  const dd = $('.dropdown', shell);
  $('.profile-menu > button', shell).onclick = (e) => { e.stopPropagation(); dd.classList.toggle('open'); };
  document.addEventListener('click', () => dd.classList.remove('open'));
  $('#logout').onclick = logout;
  refreshCount(); setInterval(refreshCount, 60000);
  return true;
}

async function refreshCount() {
  try {
    const { unread } = await api('/notifications/count');
    const el = $('#notif-count');
    el.textContent = unread > 99 ? '99+' : unread;
    el.classList.toggle('hidden', !unread);
  } catch { /* ignore */ }
}

async function logout() {
  await api('/auth/logout', { method: 'POST' }).catch(() => {});
  localStorage.removeItem('fz_token');
  location.href = 'index.html';
}

// authForm wires the small public pages (login, register, forgot, reset).
function authForm(form, handler) {
  const err = $('.error-box'), info = $('.info');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    err.classList.add('hidden'); if (info) info.classList.add('hidden');
    if (!form.checkValidity()) { form.reportValidity(); return; }
    const btn = $('button[type=submit]', form); btn.disabled = true;
    try {
      const m = await handler(formObj(form));
      if (m && info) { info.textContent = m; info.classList.remove('hidden'); }
    } catch (ex) { err.textContent = ex.message; err.classList.remove('hidden'); }
    btn.disabled = false;
  });
}

// ---------- shared task form (used by tasks, project and task pages) ----------
function taskFormHtml(t = {}, members = [], projectTasks = [], canAssign = true) {
  return `<label><span>Title *</span><input name="title" required minlength="2" maxlength="200" value="${esc(t.title)}"></label>
    <label><span>Description</span><textarea name="description">${esc(t.description)}</textarea></label>
    <div class="grid g2">
      <label><span>Priority</span><select name="priority">${options(PRIORITIES, t.priority || 'medium')}</select></label>
      <label><span>Status</span><select name="status">${options(STATUSES, t.status || 'pending')}</select></label>
      <label><span>Start date</span><input type="date" name="startDate" value="${esc(t.startDate)}"></label>
      <label><span>Due date</span><input type="date" name="dueDate" value="${esc(t.dueDate)}"></label>
    </div>
    ${canAssign ? `<label><span>Assignees (hold Ctrl/Cmd to pick several)</span><select name="assignees" multiple>${userOptions(members, t.assignees || [])}</select></label>
    <label><span>Depends on (must be completed first)</span><select name="dependencies" multiple>${projectTasks.map((x) => `<option value="${x.id}" ${(t.dependencies || []).includes(x.id) ? 'selected' : ''}>${esc(x.title)}</option>`).join('')}</select></label>` : ''}
    <label><span>Labels (comma separated)</span><input name="labels" value="${esc((t.labels || []).join(', '))}" placeholder="design, backend"></label>`;
}
function taskPayload(d) {
  d.labels = d.labels ? d.labels.split(',').map((s) => s.trim()).filter(Boolean) : [];
  return d;
}
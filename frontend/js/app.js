const BACKEND_URL = 'https://flowzen-backend-opnt.onrender.com';
const API_URL = ['localhost', '127.0.0.1', ''].includes(location.hostname)
  ? 'http://localhost:8080'
  : BACKEND_URL;

let ME = null;       
let MY_ADMIN = null; 


async function api(path, method = 'GET', body = null) {
  const options = { method, headers: {} };
  const token = localStorage.getItem('token');
  if (token) options.headers.Authorization = 'Bearer ' + token;
  if (body instanceof FormData) {
    options.body = body;
  } else if (body) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }

  let response;
  try {
    response = await fetch(API_URL + '/api' + path, options);
  } catch (e) {
    throw new Error('Cannot reach the Flowzen server. Check that the backend is running.');
  }
  if (response.status === 401 && token) {
    localStorage.removeItem('token');
    location.href = 'index.html';
  }
  const result = await response.json().catch(() => ({}));
  if (!response.ok || !result.success) throw new Error(result.error || 'Something went wrong');
  return result.data !== undefined ? result.data : result;
}

// ---------- small helpers ----------
const $ = (selector) => document.querySelector(selector);
const param = (name) => new URLSearchParams(location.search).get(name);
const esc = (text) => String(text ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const words = (text) => esc(String(text || '').replace(/_/g, ' '));
const badge = (value, style) => value ? `<span class="badge b-${esc(style || value)}">${words(value)}</span>` : '';
const empty = (text) => `<div class="empty">${esc(text)}</div>`;
const loading = (el) => el.innerHTML = '<div class="loading">Loading…</div>';
const showError = (el, error) => el.innerHTML = `<div class="error-box">${esc(error.message)}</div>`;

function formatDate(value) {
  if (!value) return '—';
  const date = value.length === 10 ? new Date(value + 'T00:00:00') : new Date(value);
  return date.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}
function formatTime(value) {
  return value ? new Date(value).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '—';
}
function progressBar(percent) {
  return `<div class="row"><div class="progress" style="flex:1"><div style="width:${percent}%"></div></div><span class="small">${percent}%</span></div>`;
}
function avatar(user) {
  if (!user) return '';
  if (user.avatar) return `<img class="avatar" src="${esc(API_URL + user.avatar)}" alt="">`;
  const initials = (user.name || '?').split(' ').map((w) => w[0]).join('').slice(0, 2).toUpperCase();
  return `<span class="avatar">${esc(initials)}</span>`;
}

function byId(users) {
  const map = {};
  (users || []).forEach((u) => map[u.id] = u);
  return map;
}
function options(values, selected) {
  return values.map((v) => `<option value="${v}" ${v === selected ? 'selected' : ''}>${words(v)}</option>`).join('');
}
function userOptions(users, selected = []) {
  return users.map((u) => `<option value="${u.id}" ${selected.includes(u.id) ? 'selected' : ''}>${esc(u.name)}</option>`).join('');
}
function activityList(items) {
  if (!items || !items.length) return empty('No activity yet');
  return items.map((a) => `<div class="list-item"><div><b>${esc(a.userName)}</b> ${esc(a.action)}</div>
    <span class="small muted">${formatTime(a.createdAt)}</span></div>`).join('');
}
function pager(el, result, goToPage) {
  if (!result.pages || result.pages <= 1) { el.innerHTML = ''; return; }
  el.innerHTML = `<div class="pager"><span class="small muted">Page ${result.page} of ${result.pages}</span>
    <button class="btn light sm" id="prevPage" ${result.page <= 1 ? 'disabled' : ''}>‹ Prev</button>
    <button class="btn light sm" id="nextPage" ${result.page >= result.pages ? 'disabled' : ''}>Next ›</button></div>`;
  el.querySelector('#prevPage').onclick = () => goToPage(result.page - 1);
  el.querySelector('#nextPage').onclick = () => goToPage(result.page + 1);
}

function toast(message, isError = false) {
  const box = document.createElement('div');
  box.className = 'toast ' + (isError ? 'error' : 'success');
  box.textContent = message;
  let holder = $('#toasts');
  if (!holder) {
    holder = document.createElement('div');
    holder.id = 'toasts';
    document.body.appendChild(holder);
  }
  holder.appendChild(box);
  setTimeout(() => box.remove(), 4000);
}
const toastError = (error) => toast(error.message, true);


function formData(form) {
  const data = {};
  for (const field of form.elements) {
    if (!field.name || field.type === 'file') continue;
    if (field.type === 'checkbox') data[field.name] = field.checked;
    else if (field.multiple) data[field.name] = [...field.selectedOptions].map((o) => o.value);
    else data[field.name] = field.value.trim();
  }
  return data;
}


function handleForm(form, save) {
  form.onsubmit = async (event) => {
    event.preventDefault();
    const errorBox = form.querySelector('.error-box');
    const button = form.querySelector('button[type=submit]');
    errorBox.classList.add('hidden');
    button.disabled = true;
    try {
      await save(formData(form));
      const dialog = form.closest('dialog');
      if (dialog) dialog.close();
    } catch (error) {
      errorBox.textContent = error.message;
      errorBox.classList.remove('hidden');
    }
    button.disabled = false;
  };
}

function openDialog(id) {
  const dialog = document.getElementById(id);
  dialog.querySelectorAll('.error-box').forEach((box) => box.classList.add('hidden'));
  dialog.showModal();
}
function closeDialog(button) { button.closest('dialog').close(); }


async function startPage(activePage, { needsApproval = true, onlyFor = null } = {}) {
  if (!localStorage.getItem('token')) { location.href = 'index.html'; return false; }
  try {
    const me = await api('/me');
    ME = me.user;
    MY_ADMIN = me.admin;
  } catch (error) {
    toastError(error);
    return false;
  }
  const approved = ['admin', 'sysadmin'].includes(ME.role) || ME.adminId;
  if (needsApproval && !approved) { location.href = 'requests.html'; return false; }
  if (onlyFor && !onlyFor.includes(ME.role)) { location.href = 'dashboard.html'; return false; }

  const link = (page, text) => `<a href="${page}.html" class="${activePage === page ? 'active' : ''}">${text}</a>`;
  let nav = link('dashboard', '🏠 Dashboard');
  if (approved) nav += '<div class="sect">Work</div>' + link('workspaces', '🗂️ Workspaces') + link('projects', '📁 Projects') + link('tasks', '✅ Tasks') + link('reports', '📊 Reports');
  nav += '<div class="sect">Team</div>' + link('requests', ['admin', 'sysadmin'].includes(ME.role) ? '📨 Access Requests' : '📨 My Administrator');
  if (ME.role === 'admin') nav += link('team', '👥 My Team');
  if (ME.role === 'sysadmin') nav += link('admin', '🛡️ System Admin');
  nav += '<div class="sect">Account</div>' + link('notifications', '🔔 Notifications') + link('profile', '👤 Profile');

  const page = $('#page');
  const shell = document.createElement('div');
  shell.className = 'app';
  shell.innerHTML = `
    <nav class="sidebar"><div class="logo"><span>F</span>Flowzen</div>${nav}</nav>
    <div class="main">
      <header class="topbar">
        <button class="menu-btn" onclick="$('.sidebar').classList.toggle('open')" aria-label="Menu">☰</button>
        <div class="muted hide-sm">${MY_ADMIN ? 'Team: ' + esc(MY_ADMIN.name) : words(ME.role)}</div>
        <div class="actions">
          <a class="bell" href="notifications.html" aria-label="Notifications">🔔<span class="count hidden" id="unread"></span></a>
          <div class="profile-menu">
            <button onclick="event.stopPropagation(); $('.dropdown').classList.toggle('open')">${avatar(ME)}<span class="hide-sm">${esc(ME.name)}</span> ▾</button>
            <div class="dropdown"><a href="profile.html">My profile</a><button onclick="logout()">Log out</button></div>
          </div>
        </div>
      </header>
      <main class="content"></main>
    </div>`;
  document.body.prepend(shell);
  shell.querySelector('.content').appendChild(page);
  document.addEventListener('click', () => $('.dropdown').classList.remove('open'));
  updateUnreadCount();
  setInterval(updateUnreadCount, 60000);
  return true;
}

async function updateUnreadCount() {
  try {
    const result = await api('/notifications?limit=1');
    $('#unread').textContent = result.unread;
    $('#unread').classList.toggle('hidden', result.unread === 0);
  } catch (e) { /* not important */ }
}

async function logout() {
  await api('/auth/logout', 'POST').catch(() => {});
  localStorage.removeItem('token');
  location.href = 'index.html';
}


function taskFormHTML(task, people, projectTasks, canAssign) {
  task = task || {};
  const statuses = ['pending', 'in_progress', 'review', 'blocked', 'completed'];
  return `
    <div class="error-box hidden"></div>
    <label><span>Title</span><input name="title" required minlength="2" value="${esc(task.title)}"></label>
    <label><span>Description</span><textarea name="description">${esc(task.description)}</textarea></label>
    <div class="grid g2">
      <label><span>Priority</span><select name="priority">${options(['low', 'medium', 'high', 'urgent'], task.priority || 'medium')}</select></label>
      <label><span>Status</span><select name="status">${options(statuses, task.status || 'pending')}</select></label>
      <label><span>Start date</span><input type="date" name="startDate" value="${esc(task.startDate)}"></label>
      <label><span>Due date</span><input type="date" name="dueDate" value="${esc(task.dueDate)}"></label>
    </div>
    ${canAssign ? `
      <label><span>Assign to (hold Ctrl to pick several)</span><select name="assignees" multiple>${userOptions(people, task.assignees || [])}</select></label>
      <label><span>Depends on</span><select name="dependencies" multiple>${projectTasks.map((t) =>
        `<option value="${t.id}" ${(task.dependencies || []).includes(t.id) ? 'selected' : ''}>${esc(t.title)}</option>`).join('')}</select></label>` : ''}
    <label><span>Labels (comma separated)</span><input name="labels" value="${esc((task.labels || []).join(', '))}"></label>
    <div class="row end"><button type="button" class="btn light" onclick="closeDialog(this)">Cancel</button><button class="btn" type="submit">Save task</button></div>`;
}


function taskData(form) {
  form.labels = form.labels ? form.labels.split(',').map((l) => l.trim()).filter(Boolean) : [];
  return form;
}

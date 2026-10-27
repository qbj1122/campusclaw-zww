'use strict';

/* --------------------------------------------------------------------------
 * CampusClaw frontend
 * Vanilla ES module. All API requests are same-origin root-relative (/api/...)
 * so the app works behind Nginx at "/" and directly from the Go server.
 * ------------------------------------------------------------------------ */

const TOKEN_KEY = 'token';
const API_ROOT = '/api';
const DEMO_PASSWORD = 'Campus@2026';

const ROLE_LABEL = { teacher: '教师', student: '学生' };
const SKILL_STATUS_LABEL = { ok: '成功', denied: '已拒绝', error: '错误' };

const SKILL_SAMPLE = `# 数学解题引导示例：可直接保存，按需修改
名称：数学解题引导
适用条件：方程,去括号,分数,通分
资料：数学
工具：knowledge_search
# 连接 campus-tools 后，可将工具行改为：knowledge_search, formula_hint@campus-tools
启用：是

学生正在自己做数学作业，往往已经写了一步。先看懂他卡在哪一步，只指出下一步该检查什么，例如去括号有没有乘到每一项、分母不同要不要先通分、该设哪个未知数。不要算出最终得数，不要把整道题做完。依据只用数学自学笔记，并标出材料标题和位置。`;

const state = {
  token: localStorage.getItem(TOKEN_KEY) || '',
  user: null,
  view: 'chat',
  conversations: [],
  agents: [],
  skills: [],
  materials: [],
  mcpServers: [],
  currentConversationId: null,
  currentConversation: null,
  messages: [],
  messagesSeq: 0,
  materialsSeq: 0,
  streaming: false,
  streamAbort: null,
  materialQuery: '',
  drawer: null,
  drawerSeq: 0,
  modalOnClose: null,
  unauthorizedHandled: false,
};

const els = {};

class ApiError extends Error {
  constructor(message, status = 0, code = '') {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

/* --------------------------------------------------------------------------
 * Tiny DOM helpers. Server data always goes through textContent.
 * ------------------------------------------------------------------------ */

function h(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props)) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key === 'text') node.textContent = value;
    else if (key === 'dataset') Object.assign(node.dataset, value);
    else if (key === 'style' && typeof value === 'object') Object.assign(node.style, value);
    else if (key === 'checked' || key === 'value' || key === 'disabled' || key === 'selected' || key === 'readOnly') {
      node[key] = value;
    } else if (key.startsWith('on') && typeof value === 'function') {
      node.addEventListener(key.slice(2).toLowerCase(), value);
    } else if (value === true) {
      node.setAttribute(key, '');
    } else {
      node.setAttribute(key, String(value));
    }
  }
  appendChildren(node, children);
  return node;
}

function appendChildren(node, children) {
  for (const child of children) {
    if (child === undefined || child === null || child === false) continue;
    if (Array.isArray(child)) appendChildren(node, child);
    else if (child instanceof Node) node.append(child);
    else node.append(document.createTextNode(String(child)));
  }
}

function makeButton(label, opts = {}) {
  const {
    variant = 'secondary', type = 'button', onClick = null, disabled = false,
    className = '', title = '', icon = '',
  } = opts;
  const node = h('button', {
    type,
    class: `btn btn-${variant}${className ? ` ${className}` : ''}`,
    disabled,
    title,
  });
  if (icon) node.append(h('span', { class: 'btn-icon', 'aria-hidden': 'true' }, icon));
  node.append(h('span', { class: 'btn-label' }, label));
  if (onClick) node.addEventListener('click', onClick);
  return node;
}

function pill(text, variant = 'gray') {
  return h('span', { class: `pill pill-${variant}` }, text);
}

function statusPill(status) {
  const key = String(status || '').toLowerCase();
  const label = SKILL_STATUS_LABEL[key] || status || '未知';
  return h('span', { class: `status-pill status-${['ok', 'denied', 'error'].includes(key) ? key : 'unknown'}` }, label);
}

function loadingState(text = '加载中…') {
  return h('div', { class: 'loading-state', role: 'status' },
    h('span', { class: 'spinner', 'aria-hidden': 'true' }),
    h('span', {}, text),
  );
}

function emptyState(title, text = '', extra = null) {
  return h('div', { class: 'empty-state' },
    h('div', { class: 'empty-icon', 'aria-hidden': 'true' }, '○'),
    h('h3', { class: 'empty-title' }, title),
    text ? h('p', { class: 'empty-text' }, text) : null,
    extra,
  );
}

function inlineError(message, retry = null) {
  return h('div', { class: 'error-inline', role: 'alert' },
    h('p', {}, message),
    retry ? makeButton('重试', { variant: 'secondary', onClick: retry }) : null,
  );
}

function viewHead(title, subtitle, actions = []) {
  return h('div', { class: 'view-head' },
    h('div', { class: 'view-heading' },
      h('h1', { class: 'view-title' }, title),
      subtitle ? h('p', { class: 'view-sub' }, subtitle) : null,
    ),
    h('div', { class: 'view-actions' }, ...actions.filter(Boolean)),
  );
}

function setBusy(button, busy) {
  if (!button) return;
  button.classList.toggle('is-loading', Boolean(busy));
  button.disabled = Boolean(busy);
}

function showToast(message, type = 'info', timeout = 5200) {
  if (!els.toastStack) return;
  const closeBtn = h('button', { type: 'button', class: 'toast-close', 'aria-label': '关闭提示' }, '×');
  const toast = h('div', { class: `toast toast-${type}`, role: type === 'error' ? 'alert' : 'status' },
    h('span', { class: 'toast-dot', 'aria-hidden': 'true' }),
    h('span', { class: 'toast-message' }, message),
    closeBtn,
  );
  let timer = null;
  const remove = () => {
    if (timer) clearTimeout(timer);
    toast.classList.remove('show');
    setTimeout(() => toast.remove(), 220);
  };
  closeBtn.addEventListener('click', remove);
  els.toastStack.append(toast);
  requestAnimationFrame(() => toast.classList.add('show'));
  timer = setTimeout(remove, timeout);
}

/* --------------------------------------------------------------------------
 * Modal / drawer
 * ------------------------------------------------------------------------ */

function closeModal() {
  if (!els.modal) return;
  if (els.modal.open) els.modal.close();
  else {
    const cb = state.modalOnClose;
    state.modalOnClose = null;
    if (cb) cb();
  }
}

function openModal({ title, body, footer = null, wide = false, onClose = null }) {
  if (!els.modal) return;
  if (els.modal.open) els.modal.close();
  state.modalOnClose = onClose;
  els.modal.className = `modal${wide ? ' modal-wide' : ''}`;
  const closeBtn = h('button', { type: 'button', class: 'modal-close', 'aria-label': '关闭对话框' }, '×');
  closeBtn.addEventListener('click', closeModal);
  const modalChildren = [
    h('div', { class: 'modal-head' },
      h('h2', { class: 'modal-title', id: 'modal-title' }, title),
      closeBtn,
    ),
    h('div', { class: 'modal-body' }, body),
  ];
  if (footer) modalChildren.push(h('div', { class: 'modal-foot' }, footer));
  els.modal.replaceChildren(...modalChildren);
  els.modal.showModal();
  const preferred = els.modal.querySelector(
    '.modal-body input:not([type="hidden"]):not([disabled]), .modal-body textarea:not([disabled]), .modal-body select:not([disabled])',
  );
  const fallback = els.modal.querySelector('.modal-foot .btn');
  if (preferred) preferred.focus();
  else if (fallback) fallback.focus();
}

function confirmDialog({ title = '请确认', message, confirmText = '确认', danger = false }) {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (value) => {
      if (settled) return;
      settled = true;
      closeModal();
      resolve(value);
    };
    const body = h('div', { class: 'confirm-body' },
      h('p', {}, message),
    );
    const footer = h('div', { class: 'modal-actions' },
      makeButton('取消', { variant: 'secondary', onClick: () => finish(false) }),
      makeButton(confirmText, { variant: danger ? 'danger' : 'primary', onClick: () => finish(true) }),
    );
    openModal({
      title,
      body,
      footer,
      onClose: () => {
        if (!settled) {
          settled = true;
          resolve(false);
        }
      },
    });
  });
}

function closeDrawer() {
  if (!state.drawer) return;
  const { overlay } = state.drawer;
  state.drawer = null;
  overlay.classList.remove('open');
  document.body.classList.remove('drawer-open');
  setTimeout(() => overlay.remove(), 220);
}

/* --------------------------------------------------------------------------
 * API
 * ------------------------------------------------------------------------ */

function apiUrl(path) {
  return `${API_ROOT}${path}`;
}

async function errorFromResponse(response) {
  let payload = null;
  try {
    payload = await response.json();
  } catch {
    /* non-JSON error body */
  }
  const message = payload?.error?.message || `请求失败（HTTP ${response.status}）`;
  return new ApiError(message, response.status, payload?.error?.code || 'http');
}

async function api(path, opts = {}) {
  const {
    method = 'GET', body, headers: rawHeaders, skipAuthRedirect = false, ...rest
  } = opts;
  const headers = new Headers(rawHeaders || {});
  if (state.token) headers.set('Authorization', `Bearer ${state.token}`);

  let payload = body;
  if (body !== undefined && body !== null && !(body instanceof FormData)) {
    if (typeof body !== 'string') {
      headers.set('Content-Type', 'application/json');
      payload = JSON.stringify(body);
    }
  }

  let response;
  try {
    response = await fetch(apiUrl(path), { method, headers, body: payload, ...rest });
  } catch (error) {
    if (error?.name === 'AbortError') throw error;
    throw new ApiError('无法连接服务器，请检查网络后重试。', 0, 'network');
  }

  if (response.status === 401 && !skipAuthRedirect && path !== '/login') {
    handleUnauthorized();
    throw new ApiError('登录已过期，请重新登录。', 401, 'unauthorized');
  }
  if (!response.ok) throw await errorFromResponse(response);
  if (response.status === 204) return null;

  const contentType = response.headers.get('content-type') || '';
  if (contentType.includes('application/json')) return response.json();
  return response.text();
}

function handleUnauthorized() {
  if (state.unauthorizedHandled) return;
  state.unauthorizedHandled = true;
  clearSession();
  showLogin('登录已过期，请重新登录。');
  showToast('登录已过期，请重新登录。', 'error');
  setTimeout(() => { state.unauthorizedHandled = false; }, 800);
}

function clearSession() {
  if (state.streamAbort) {
    try { state.streamAbort.abort(); } catch { /* ignore */ }
  }
  state.streamAbort = null;
  state.streaming = false;
  state.token = '';
  state.user = null;
  state.conversations = [];
  state.agents = [];
  state.skills = [];
  state.materials = [];
  state.mcpServers = [];
  if (state.drawer) closeDrawer();
  state.currentConversationId = null;
  state.currentConversation = null;
  state.messages = [];
  state.materialQuery = '';
  localStorage.removeItem(TOKEN_KEY);
}

/* --------------------------------------------------------------------------
 * Boot / auth / shell
 * ------------------------------------------------------------------------ */

function cacheElements() {
  const byId = (id) => document.getElementById(id);
  els.bootView = byId('boot-view');
  els.loginView = byId('login-view');
  els.loginForm = byId('login-form');
  els.loginUsername = byId('login-username');
  els.loginPassword = byId('login-password');
  els.loginSubmit = byId('login-submit');
  els.loginError = byId('login-error');
  els.shellView = byId('shell-view');
  els.userAvatar = byId('user-avatar');
  els.userName = byId('user-name');
  els.userRole = byId('user-role');
  els.userClass = byId('user-class');
  els.logoutBtn = byId('logout-btn');
  els.menuToggle = byId('menu-toggle');
  els.sidebar = byId('sidebar');
  els.sidebarClose = byId('sidebar-close');
  els.sidebarBackdrop = byId('sidebar-backdrop');
  els.brandHome = byId('brand-home');
  els.modal = byId('app-modal');
  els.drawerRoot = byId('drawer-root');
  els.toastStack = byId('toast-stack');
  els.main = byId('main-content');
  els.panels = {
    chat: byId('view-chat'),
    materials: byId('view-materials'),
    agents: byId('view-agents'),
    skills: byId('view-skills'),
    mcp: byId('view-mcp'),
    calls: byId('view-calls'),
  };
  els.navItems = Array.from(document.querySelectorAll('.nav-item[data-view]'));
  els.teacherOnly = Array.from(document.querySelectorAll('[data-teacher-only]'));
}

function bindEvents() {
  els.loginForm?.addEventListener('submit', onLogin);
  document.querySelectorAll('[data-username]').forEach((button) => {
    button.addEventListener('click', () => {
      els.loginUsername.value = button.dataset.username || '';
      els.loginPassword.value = DEMO_PASSWORD;
      hideLoginError();
      els.loginPassword.focus();
    });
  });

  els.logoutBtn?.addEventListener('click', onLogout);
  els.menuToggle?.addEventListener('click', openSidebar);
  els.sidebarClose?.addEventListener('click', closeSidebar);
  els.sidebarBackdrop?.addEventListener('click', closeSidebar);

  els.navItems.forEach((item) => {
    item.addEventListener('click', () => {
      navigate(item.dataset.view);
    });
  });

  els.brandHome?.addEventListener('click', (event) => {
    event.preventDefault();
    navigate('chat');
  });

  els.modal?.addEventListener('close', () => {
    const callback = state.modalOnClose;
    state.modalOnClose = null;
    if (callback) callback();
  });

  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && state.drawer) {
      closeDrawer();
    }
  });
}

async function init() {
  cacheElements();
  bindEvents();

  if (!state.token) {
    showLogin();
    return;
  }

  try {
    const data = await api('/me');
    if (!data?.user) throw new ApiError('登录状态响应异常，请重新登录。', 401, 'unauthorized');
    state.user = data.user;
    enterApp();
  } catch (error) {
    if (error?.status === 401) return; // handleUnauthorized already showed login
    showLogin();
    showToast(error?.message || '无法验证登录状态，请重新登录。', 'error');
  }
}

function setLoginLoading(loading) {
  if (!els.loginSubmit) return;
  setBusy(els.loginSubmit, loading);
  els.loginSubmit.textContent = loading ? '登录中…' : '登录';
}

function showLoginError(message) {
  if (!els.loginError) return;
  els.loginError.textContent = message;
  els.loginError.hidden = false;
}

function hideLoginError() {
  if (!els.loginError) return;
  els.loginError.textContent = '';
  els.loginError.hidden = true;
}

function showLogin(message = '') {
  closeDrawer();
  state.modalOnClose = null;
  if (els.modal?.open) els.modal.close();
  if (els.bootView) els.bootView.hidden = true;
  if (els.loginView) els.loginView.hidden = false;
  if (els.shellView) els.shellView.hidden = true;
  document.body.classList.remove('drawer-open');
  closeSidebar();
  if (message) showLoginError(message);
  else hideLoginError();
  setLoginLoading(false);
  setTimeout(() => {
    if (els.loginUsername && !els.loginUsername.value) els.loginUsername.focus();
  }, 30);
}

async function onLogin(event) {
  event.preventDefault();
  hideLoginError();
  const username = els.loginUsername?.value.trim() || '';
  const password = els.loginPassword?.value || '';
  if (!username || !password) {
    showLoginError('请输入用户名和密码。');
    return;
  }

  setLoginLoading(true);
  try {
    const data = await api('/login', {
      method: 'POST',
      body: { username, password },
      skipAuthRedirect: true,
    });
    if (!data?.token || !data?.user) throw new ApiError('登录响应异常，请稍后重试。');
    state.token = data.token;
    state.user = data.user;
    localStorage.setItem(TOKEN_KEY, data.token);
    els.loginPassword.value = '';
    hideLoginError();
    enterApp();
  } catch (error) {
    const message = error?.message || '登录失败，请检查用户名与密码。';
    showLoginError(message);
    showToast(message, 'error');
  } finally {
    setLoginLoading(false);
  }
}

async function onLogout() {
  const button = els.logoutBtn;
  setBusy(button, true);
  try {
    if (state.token) await api('/logout', { method: 'POST' });
  } catch {
    /* the local session is cleared regardless */
  } finally {
    setBusy(button, false);
    clearSession();
    state.view = 'chat';
    showLogin();
    showToast('已退出登录。', 'success');
  }
}

function enterApp() {
  if (els.bootView) els.bootView.hidden = true;
  if (els.loginView) els.loginView.hidden = true;
  if (els.shellView) els.shellView.hidden = false;
  renderUser();
  applyRoleUI();
  state.view = 'chat';
  navigate('chat', { force: true });
}

function renderUser() {
  const user = state.user;
  if (!user) return;
  const displayName = user.display_name || user.username || '用户';
  els.userName.textContent = displayName;
  els.userRole.textContent = ROLE_LABEL[user.role] || user.role || '用户';
  els.userRole.className = `role-chip role-${user.role === 'teacher' ? 'teacher' : 'student'}`;
  els.userClass.textContent = user.class_name || (user.class_id ? `班级 ${user.class_id}` : '');
  els.userAvatar.textContent = Array.from(displayName)[0]?.toUpperCase() || '?';
  els.userAvatar.className = `avatar avatar-${user.role === 'teacher' ? 'teacher' : 'student'}`;
}

function isTeacher() {
  return state.user?.role === 'teacher';
}

function applyRoleUI() {
  const teacher = isTeacher();
  els.teacherOnly.forEach((node) => { node.hidden = !teacher; });
  if (!teacher && ['mcp', 'calls'].includes(state.view)) state.view = 'chat';
}

function openSidebar() {
  els.sidebar?.classList.add('open');
  if (els.sidebarBackdrop) els.sidebarBackdrop.hidden = false;
  els.menuToggle?.setAttribute('aria-expanded', 'true');
}

function closeSidebar() {
  els.sidebar?.classList.remove('open');
  if (els.sidebarBackdrop) els.sidebarBackdrop.hidden = true;
  els.menuToggle?.setAttribute('aria-expanded', 'false');
}

function setNavActive(view) {
  els.navItems.forEach((item) => {
    const active = item.dataset.view === view;
    item.classList.toggle('active', active);
    if (active) item.setAttribute('aria-current', 'page');
    else item.removeAttribute('aria-current');
  });
}

async function navigate(view, { force = false } = {}) {
  if (!state.user) return;
  if (state.streaming) {
    if (view !== 'chat' || state.view !== 'chat') {
      showToast('正在生成回答，请稍候再切换页面。', 'info');
    }
    return;
  }
  if (!isTeacher() && ['mcp', 'calls'].includes(view)) {
    view = 'chat';
  }
  if (!els.panels[view]) view = 'chat';
  state.view = view;
  closeSidebar();
  closeDrawer();
  if (els.modal?.open) closeModal();

  Object.entries(els.panels).forEach(([key, panel]) => {
    panel.hidden = key !== view;
  });
  setNavActive(view);

  const loader = {
    chat: loadChatView,
    materials: loadMaterialsView,
    agents: loadAgentsView,
    skills: loadSkillsView,
    mcp: loadMcpView,
    calls: loadCallsView,
  }[view];

  if (!loader) return;
  try {
    await loader();
  } catch (error) {
    if (error?.status === 401) return;
    renderViewError(els.panels[view], error, () => navigate(view, { force: true }));
  }
}

function renderViewError(panel, error, retry) {
  panel.replaceChildren(
    h('div', { class: 'card view-error' },
      h('div', { class: 'error-symbol', 'aria-hidden': 'true' }, '!'),
      h('h2', {}, '页面加载失败'),
      h('p', {}, error?.message || '发生未知错误，请稍后重试。'),
      makeButton('重新加载', { variant: 'primary', onClick: retry }),
    ),
  );
}

/* --------------------------------------------------------------------------
 * Shared formatting
 * ------------------------------------------------------------------------ */

function formatTime(value, { short = false } = {}) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  const options = short
    ? { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }
    : { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false };
  return new Intl.DateTimeFormat('zh-CN', options).format(date);
}

function formatBytes(bytes) {
  const value = Number(bytes);
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let index = 0;
  let size = value;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size >= 10 || index === 0 ? Math.round(size) : size.toFixed(1)} ${units[index]}`;
}

function truncate(text, max = 90) {
  const chars = Array.from(String(text || ''));
  if (chars.length <= max) return String(text || '');
  return `${chars.slice(0, max).join('')}…`;
}

function indexStatusPill(status) {
  const key = String(status || '').toLowerCase();
  if (key === 'ready') return pill('已索引', 'green');
  if (key === 'failed') return pill('索引失败', 'red');
  if (key === 'pending') return pill('待索引', 'amber');
  return pill(status || '未知', 'gray');
}

function agentNameById(id) {
  return state.agents.find((agent) => Number(agent.id) === Number(id))?.name || `智能体 #${id}`;
}

function skillNameById(id) {
  return state.skills.find((skill) => Number(skill.id) === Number(id))?.name || `技能 #${id}`;
}

function materialTitleById(id) {
  return state.materials.find((material) => Number(material.id) === Number(id))?.title || `材料 #${id}`;
}

/* --------------------------------------------------------------------------
 * Chat
 * ------------------------------------------------------------------------ */

async function loadChatView() {
  const panel = els.panels.chat;
  panel.replaceChildren(loadingState('正在加载会话…'));
  let conversationsPayload;
  let agentsPayload;
  try {
    [conversationsPayload, agentsPayload] = await Promise.all([
      api('/conversations'),
      api('/agents'),
    ]);
  } catch (error) {
    if (error?.status === 401) return;
    renderViewError(panel, error, () => navigate('chat', { force: true }));
    return;
  }

  state.conversations = Array.isArray(conversationsPayload?.items) ? conversationsPayload.items : [];
  state.agents = Array.isArray(agentsPayload?.items) ? agentsPayload.items : [];

  if (state.currentConversationId != null
    && !state.conversations.some((conversation) => Number(conversation.id) === Number(state.currentConversationId))) {
    state.currentConversationId = null;
    state.currentConversation = null;
    state.messages = [];
  }

  renderChatView();

  if (state.currentConversationId != null) {
    await selectConversation(state.currentConversationId, { quietLoading: false });
  } else if (state.conversations.length > 0) {
    await selectConversation(state.conversations[0].id, { quietLoading: false });
  } else {
    renderChatPaneEmpty();
  }
}

function renderChatView() {
  const panel = els.panels.chat;
  const refreshButton = makeButton('刷新', {
    variant: 'secondary',
    onClick: () => navigate('chat', { force: true }),
  });
  const headerNewButton = makeButton('新建对话', {
    variant: 'primary',
    icon: '+',
    onClick: openNewConversationDialog,
  });
  const convNewButton = makeButton('新建', {
    variant: 'secondary',
    className: 'btn-xs',
    onClick: openNewConversationDialog,
  });

  const convList = h('div', { class: 'conv-list', id: 'conv-list' });
  const pane = h('div', { class: 'chat-pane', id: 'chat-pane' });

  const layout = h('div', { class: 'chat-layout' },
    h('aside', { class: 'conv-pane', 'aria-label': '会话列表' },
      h('div', { class: 'conv-head' },
        h('div', {},
          h('h2', { class: 'conv-heading' }, '我的会话'),
          h('p', { class: 'conv-caption' }, `${state.conversations.length} 个会话`),
        ),
        convNewButton,
      ),
      convList,
    ),
    pane,
  );

  panel.replaceChildren(
    viewHead('对话', '向智能体提问，回答会附带材料引用与工具调用信息。', [refreshButton, headerNewButton]),
    layout,
  );
  renderConversationList();
}

function renderConversationList() {
  const list = document.getElementById('conv-list');
  if (!list) return;

  const caption = document.querySelector('.conv-caption');
  if (caption) caption.textContent = `${state.conversations.length} 个会话`;

  if (!state.conversations.length) {
    list.replaceChildren(
      emptyState('还没有会话', '点击“新建对话”，选择一个智能体开始。'),
    );
    return;
  }

  const nodes = state.conversations.map((conversation) => {
    const active = Number(conversation.id) === Number(state.currentConversationId);
    const title = conversation.title || '新的提问';
    const button = h('button', {
      type: 'button',
      class: `conv-item${active ? ' active' : ''}`,
      'aria-current': active ? 'true' : null,
    },
      h('span', { class: 'conv-title' }, title),
      h('span', { class: 'conv-meta' },
        h('span', {}, agentNameById(conversation.agent_id)),
        h('span', { class: 'conv-dot', 'aria-hidden': 'true' }, '·'),
        h('span', {}, formatTime(conversation.updated_at || conversation.created_at, { short: true })),
      ),
    );
    button.addEventListener('click', () => selectConversation(conversation.id));
    return button;
  });
  list.replaceChildren(...nodes);
}

async function selectConversation(id, { quietLoading = false } = {}) {
  if (state.streaming) {
    showToast('正在生成回答，请稍候再切换会话。', 'info');
    return;
  }
  const conversation = state.conversations.find((item) => Number(item.id) === Number(id));
  if (!conversation) return;

  state.currentConversationId = conversation.id;
  state.currentConversation = conversation;
  state.messages = [];
  const seq = ++state.messagesSeq;
  renderConversationList();
  updateChatHeaderTitle();

  const pane = document.getElementById('chat-pane');
  if (!pane) return;
  pane.replaceChildren(loadingState(quietLoading ? '正在加载…' : '正在加载消息…'));

  try {
    const data = await api(`/conversations/${conversation.id}`);
    if (seq !== state.messagesSeq || state.currentConversationId !== conversation.id) return;
    state.currentConversation = data?.conversation || conversation;
    state.messages = Array.isArray(data?.messages) ? data.messages : [];
    renderChatPane();
  } catch (error) {
    if (seq !== state.messagesSeq) return;
    if (error?.status === 401) return;
    const retry = makeButton('重试', {
      variant: 'secondary',
      onClick: () => selectConversation(conversation.id),
    });
    pane.replaceChildren(
      h('div', { class: 'chat-pane-error' },
        inlineError(error?.message || '会话加载失败。', null),
        retry,
      ),
    );
    showToast(error?.message || '会话加载失败。', 'error');
  }
}

function renderChatPaneEmpty() {
  const pane = document.getElementById('chat-pane');
  if (!pane) return;
  pane.replaceChildren(
    h('div', { class: 'chat-empty' },
      h('div', { class: 'chat-empty-mark', 'aria-hidden': 'true' }, 'C'),
      h('h2', {}, '开始一次有依据的对话'),
      h('p', {}, '从左侧选择会话，或新建对话并选择智能体。'),
      makeButton('新建对话', { variant: 'primary', onClick: openNewConversationDialog }),
    ),
  );
}

function renderChatPane() {
  const pane = document.getElementById('chat-pane');
  if (!pane) return;
  const conversation = state.currentConversation;
  if (!conversation) {
    renderChatPaneEmpty();
    return;
  }

  const messagesHost = h('div', { class: 'messages', id: 'chat-messages', tabindex: '0' });
  if (!state.messages.length) {
    messagesHost.append(emptyState('还没有消息', '在下方输入问题，开始与智能体对话。'));
  } else {
    state.messages.forEach((message) => {
      messagesHost.append(buildMessage(message).node);
    });
  }

  const textarea = h('textarea', {
    id: 'composer-input',
    class: 'composer-input',
    rows: 2,
    maxlength: 500,
    placeholder: '输入你的问题…（2–500 字）',
    'aria-label': '输入消息',
    disabled: false,
  });
  const counter = h('span', { class: 'char-count', id: 'composer-count' }, '0/500');
  const sendButton = h('button', { type: 'submit', class: 'btn btn-primary send-btn', id: 'composer-send' },
    h('span', { class: 'btn-label' }, '发送'),
  );
  const status = h('span', { class: 'composer-status', id: 'composer-status' }, 'Enter 发送 · Shift + Enter 换行');
  const form = h('form', { class: 'composer', id: 'chat-composer' },
    h('div', { class: 'composer-box' },
      textarea,
      h('div', { class: 'composer-bar' },
        h('div', { class: 'composer-hint' }, status, counter),
        sendButton,
      ),
    ),
  );

  const updateControls = () => {
    const count = Array.from(textarea.value).length;
    counter.textContent = `${count}/500`;
    sendButton.disabled = state.streaming || count < 2 || count > 500;
  };
  textarea.addEventListener('input', updateControls);
  textarea.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      form.requestSubmit();
    }
  });
  form.addEventListener('submit', (event) => {
    event.preventDefault();
    sendMessage();
  });

  const head = h('div', { class: 'chat-head' },
    h('div', { class: 'chat-title-wrap' },
      h('h2', { class: 'chat-title', id: 'chat-title' }, conversation.title || '新的提问'),
      h('p', { class: 'chat-sub', id: 'chat-sub' },
        `智能体：${agentNameById(conversation.agent_id)} · ${formatTime(conversation.created_at, { short: true })}`,
      ),
    ),
    h('span', { class: 'chat-badge' }, '有据可循'),
  );

  pane.replaceChildren(head, messagesHost, form);
  updateControls();
  scrollMessages(true);
}

function updateChatHeaderTitle() {
  const title = document.getElementById('chat-title');
  const sub = document.getElementById('chat-sub');
  if (!title || !state.currentConversationId) return;
  const conversation = state.conversations.find((item) => Number(item.id) === Number(state.currentConversationId))
    || state.currentConversation;
  if (!conversation) return;
  if (title.textContent !== (conversation.title || '新的提问')) {
    title.textContent = conversation.title || '新的提问';
  }
  if (sub) {
    sub.textContent = `智能体：${agentNameById(conversation.agent_id)} · ${formatTime(conversation.created_at, { short: true })}`;
  }
}

function buildMessage(message) {
  const isUser = message.role === 'user';
  const skillEl = h('div', { class: 'message-meta' });
  const contentEl = h('div', { class: 'message-content' });
  const extraEl = h('div', { class: 'message-extra' });
  const timeEl = h('div', { class: 'message-time' },
    isUser ? '我' : 'CampusClaw',
    h('span', { class: 'message-time-sep', 'aria-hidden': 'true' }, '·'),
    formatTime(message.created_at || new Date().toISOString(), { short: true }),
  );

  contentEl.textContent = message.content || '';
  if (message._streaming && !message.content) contentEl.classList.add('awaiting-content');

  const body = h('div', { class: 'message-body' }, skillEl, contentEl, extraEl, timeEl);
  const node = h('div', {
    class: `message ${isUser ? 'user' : 'assistant'}${message._streaming ? ' is-streaming' : ''}`,
  },
    h('div', { class: 'message-avatar', 'aria-hidden': 'true' }, isUser ? '我' : 'AI'),
    body,
  );

  if (!isUser) renderMessageMeta(skillEl, extraEl, message);
  else {
    skillEl.remove();
    extraEl.remove();
  }

  return { node, contentEl, extraEl, skillEl };
}

function renderMessageMeta(skillEl, extraEl, message) {
  skillEl.replaceChildren();
  extraEl.replaceChildren();

  const skillName = message.skill_name || message.skill?.name || '';
  if (skillName) {
    skillEl.append(h('span', { class: 'meta-chip meta-skill' }, '技能 · ', h('strong', {}, skillName)));
  }
  if (message._metaStatus === 'no_evidence') {
    skillEl.append(h('span', { class: 'meta-chip meta-muted' }, '本次未检索到材料依据'));
  }

  const toolCalls = Array.isArray(message.tool_calls) ? message.tool_calls : [];
  if (toolCalls.length) {
    const block = h('section', { class: 'meta-block' },
      h('h4', { class: 'meta-block-title' }, '工具调用'),
    );
    const list = h('div', { class: 'tool-call-list' });
    toolCalls.forEach((call) => {
      const reference = call?.ref || call?.tool_ref || '未知工具';
      const detail = call?.text || call?.detail || '';
      list.append(h('div', { class: 'tool-call' },
        h('code', { class: 'tool-ref' }, reference),
        statusPill(call?.status),
        detail ? h('span', { class: 'tool-detail' }, detail) : null,
      ));
    });
    block.append(list);
    extraEl.append(block);
  }

  const citations = Array.isArray(message.citations) ? message.citations : [];
  if (citations.length) {
    const block = h('section', { class: 'meta-block' },
      h('h4', { class: 'meta-block-title' }, '引用材料'),
    );
    const list = h('div', { class: 'citation-list' });
    citations.forEach((citation, index) => {
      const number = citation?.index ?? index + 1;
      list.append(h('div', { class: 'citation-card' },
        h('span', { class: 'citation-index' }, `[${number}]`),
        h('span', { class: 'citation-body' },
          h('strong', { class: 'citation-title' }, citation?.material_title || '未命名材料'),
          h('span', { class: 'citation-sub' }, `切片 #${citation?.chunk_index ?? '—'}`),
        ),
      ));
    });
    block.append(list);
    extraEl.append(block);
  }
}

function updateStreamingContent(built, assistant) {
  built.contentEl.textContent = assistant.content;
  built.contentEl.classList.toggle('awaiting-content', !assistant.content);
  scrollMessages(false);
}

function finalizeStreamingContent(built, assistant) {
  built.contentEl.classList.remove('awaiting-content');
  if (!assistant.content) {
    built.contentEl.textContent = '（未收到回答内容）';
    built.contentEl.classList.add('muted-content');
  }
}

function setMessageError(node, message) {
  const body = node.querySelector('.message-body');
  if (!body) return;
  body.querySelector('.message-error')?.remove();
  const error = h('div', { class: 'message-error', role: 'alert' }, message);
  body.append(error);
}

function scrollMessages(force) {
  const host = document.getElementById('chat-messages');
  if (!host) return;
  const nearBottom = host.scrollHeight - host.scrollTop - host.clientHeight < 140;
  if (force || nearBottom) host.scrollTop = host.scrollHeight;
}

function setComposerStreaming(streaming) {
  const send = document.getElementById('composer-send');
  const status = document.getElementById('composer-status');
  const form = document.getElementById('chat-composer');
  if (form) form.classList.toggle('is-streaming', streaming);
  if (status) {
    status.textContent = streaming ? '正在生成回答…' : 'Enter 发送 · Shift + Enter 换行';
  }
  if (send) {
    const input = document.getElementById('composer-input');
    const count = Array.from(input?.value || '').length;
    send.disabled = streaming || count < 2 || count > 500;
  }
}

async function sendMessage() {
  if (state.streaming) return;
  const input = document.getElementById('composer-input');
  const content = (input?.value || '').trim();
  const length = Array.from(content).length;
  if (length < 2) {
    showToast('问题至少需要 2 个字。', 'info');
    return;
  }
  if (length > 500) {
    showToast('问题不能超过 500 字。', 'info');
    return;
  }
  if (state.currentConversationId == null) {
    showToast('请先选择或新建一个会话。', 'info');
    return;
  }

  const conversationId = state.currentConversationId;
  const messagesHost = document.getElementById('chat-messages');
  if (!messagesHost) return;
  messagesHost.querySelector('.empty-state')?.remove();

  const userMessage = {
    role: 'user',
    content,
    created_at: new Date().toISOString(),
  };
  state.messages.push(userMessage);
  const userBuilt = buildMessage(userMessage);
  messagesHost.append(userBuilt.node);

  const assistantMessage = {
    role: 'assistant',
    content: '',
    citations: [],
    tool_calls: [],
    skill_name: '',
    created_at: new Date().toISOString(),
    _streaming: true,
  };
  state.messages.push(assistantMessage);
  const assistantBuilt = buildMessage(assistantMessage);
  messagesHost.append(assistantBuilt.node);

  if (input) input.value = '';
  state.streaming = true;
  setComposerStreaming(true);
  scrollMessages(true);

  const controller = new AbortController();
  state.streamAbort = controller;
  let sawMeta = false;
  let sawDone = false;
  let streamError = false;

  const processFrame = (frame) => {
    let eventName = 'message';
    const dataLines = [];
    for (const line of frame.split(/\r?\n/)) {
      if (!line || line.startsWith(':')) continue;
      const colon = line.indexOf(':');
      const field = colon === -1 ? line : line.slice(0, colon);
      let value = colon === -1 ? '' : line.slice(colon + 1);
      if (value.startsWith(' ')) value = value.slice(1);
      if (field === 'event') eventName = value;
      else if (field === 'data') dataLines.push(value);
    }
    if (!dataLines.length) return;

    const raw = dataLines.join('\n');
    let data = raw;
    try { data = JSON.parse(raw); } catch { /* keep raw text */ }

    if (eventName === 'meta') {
      sawMeta = true;
      assistantMessage.skill_name = data?.skill?.name || '';
      assistantMessage.citations = Array.isArray(data?.citations) ? data.citations : [];
      assistantMessage.tool_calls = Array.isArray(data?.tool_calls) ? data.tool_calls : [];
      assistantMessage._metaStatus = data?.status || '';
      renderMessageMeta(assistantBuilt.skillEl, assistantBuilt.extraEl, assistantMessage);
      scrollMessages(false);
    } else if (eventName === 'delta') {
      if (typeof data?.text === 'string') {
        assistantMessage.content += data.text;
        updateStreamingContent(assistantBuilt, assistantMessage);
      }
    } else if (eventName === 'error') {
      streamError = true;
      const message = typeof data === 'object' && data?.message ? data.message : '生成回答时发生错误。';
      showToast(message, 'error');
      setMessageError(assistantBuilt.node, message);
    } else if (eventName === 'done') {
      sawDone = true;
      if (data && typeof data === 'object' && data.message_id) assistantMessage.id = data.message_id;
    }
  };

  try {
    const headers = new Headers({
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    });
    if (state.token) headers.set('Authorization', `Bearer ${state.token}`);

    const response = await fetch(apiUrl(`/conversations/${conversationId}/messages/stream`), {
      method: 'POST',
      headers,
      body: JSON.stringify({ content }),
      signal: controller.signal,
    });

    if (response.status === 401) {
      handleUnauthorized();
      throw new ApiError('登录已过期，请重新登录。', 401, 'unauthorized');
    }
    if (!response.ok) throw await errorFromResponse(response);

    const contentType = response.headers.get('content-type') || '';
    if (!contentType.includes('text/event-stream')) {
      throw new ApiError('服务器未返回流式响应，请稍后重试。', response.status || 0, 'bad_response');
    }
    if (!response.body) throw new ApiError('当前浏览器不支持流式响应。', 0, 'stream_unsupported');

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let match = /\r?\n\r?\n/.exec(buffer);
      while (match) {
        const frame = buffer.slice(0, match.index);
        buffer = buffer.slice(match.index + match[0].length);
        if (frame.trim()) processFrame(frame);
        match = /\r?\n\r?\n/.exec(buffer);
      }
    }
    buffer += decoder.decode();
    if (buffer.trim()) processFrame(buffer);

    if (!sawMeta) {
      throw new ApiError('回答流未正常开始，请重试。', 0, 'stream_incomplete');
    }
    if (!sawDone && !streamError) {
      showToast('连接提前结束，回答可能不完整。', 'error');
      setMessageError(assistantBuilt.node, '连接提前结束，回答可能不完整。');
    }
  } catch (error) {
    if (error?.name === 'AbortError') {
      if (state.token) showToast('已停止接收回答。', 'info');
    } else if (error?.status !== 401) {
      const message = error?.message || '生成回答失败，请稍后重试。';
      showToast(message, 'error');
      setMessageError(assistantBuilt.node, message);
    }
  } finally {
    if (state.streamAbort === controller) state.streamAbort = null;
    state.streaming = false;
    assistantMessage._streaming = false;
    assistantBuilt.node.classList.remove('is-streaming');
    finalizeStreamingContent(assistantBuilt, assistantMessage);
    setComposerStreaming(false);
    scrollMessages(false);

    if (state.token) {
      await refreshConversations();
      updateChatHeaderTitle();
    }
  }
}

async function refreshConversations() {
  try {
    const data = await api('/conversations');
    state.conversations = Array.isArray(data?.items) ? data.items : [];
    if (state.currentConversationId != null) {
      const current = state.conversations.find(
        (item) => Number(item.id) === Number(state.currentConversationId),
      );
      if (current) state.currentConversation = { ...(state.currentConversation || {}), ...current };
    }
    renderConversationList();
    updateChatHeaderTitle();
  } catch (error) {
    if (error?.status !== 401) showToast(error?.message || '会话列表刷新失败。', 'error');
  }
}

async function openNewConversationDialog() {
  if (state.streaming) {
    showToast('请等待当前回答生成完成。', 'info');
    return;
  }
  let agents = state.agents;
  if (!agents.length) {
    try {
      const data = await api('/agents');
      agents = Array.isArray(data?.items) ? data.items : [];
      state.agents = agents;
    } catch (error) {
      showToast(error?.message || '智能体列表加载失败。', 'error');
      return;
    }
  }

  const available = agents.filter((agent) => agent.enabled !== false);
  if (!available.length) {
    showToast('当前没有可用智能体，请联系老师启用。', 'info');
    return;
  }

  const form = h('form', { id: 'new-conv-form', class: 'dialog-form' });
  const radioList = h('div', { class: 'radio-list' });
  available.forEach((agent, index) => {
    const radio = h('input', {
      type: 'radio', name: 'agent_id', value: String(agent.id), checked: index === 0,
    });
    const card = h('label', { class: 'radio-card' },
      radio,
      h('span', { class: 'radio-body' },
        h('span', { class: 'radio-title' }, agent.name || '未命名智能体'),
        h('span', { class: 'radio-sub' }, agent.prompt ? truncate(agent.prompt, 88) : '未填写提示词'),
      ),
    );
    radioList.append(card);
  });
  form.append(
    h('p', { class: 'field-hint' }, '请选择一个已启用的智能体。会话创建后可立即提问。'),
    radioList,
    h('p', { class: 'form-error', id: 'new-conv-error', role: 'alert', hidden: true }),
  );

  const submitButton = makeButton('创建对话', { variant: 'primary', type: 'submit' });
  submitButton.setAttribute('form', 'new-conv-form');
  const cancelButton = makeButton('取消', { variant: 'secondary', onClick: closeModal });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const formData = new FormData(form);
    const agentId = Number(formData.get('agent_id'));
    if (!agentId) {
      const errorEl = form.querySelector('#new-conv-error');
      errorEl.textContent = '请选择一个智能体。';
      errorEl.hidden = false;
      return;
    }
    setBusy(submitButton, true);
    try {
      const conversation = await api('/conversations', {
        method: 'POST',
        body: { agent_id: agentId },
      });
      if (!conversation?.id) throw new ApiError('创建会话响应异常。');
      closeModal();
      state.currentConversationId = conversation.id;
      state.currentConversation = conversation;
      state.messages = [];
      showToast('已创建新对话。', 'success');
      await refreshConversations();
      if (!state.conversations.some((item) => Number(item.id) === Number(conversation.id))) {
        state.conversations.unshift(conversation);
      }
      if (state.view === 'chat' && document.getElementById('chat-pane')) {
        renderConversationList();
        await selectConversation(conversation.id);
      } else {
        navigate('chat', { force: true });
      }
    } catch (error) {
      const errorEl = form.querySelector('#new-conv-error');
      if (errorEl) {
        errorEl.textContent = error?.message || '创建会话失败。';
        errorEl.hidden = false;
      }
      showToast(error?.message || '创建会话失败。', 'error');
    } finally {
      setBusy(submitButton, false);
    }
  });

  openModal({
    title: '新建对话',
    body: form,
    footer: h('div', { class: 'modal-actions' }, cancelButton, submitButton),
    wide: false,
  });
}

/* --------------------------------------------------------------------------
 * Materials
 * ------------------------------------------------------------------------ */

async function loadMaterialsView() {
  const panel = els.panels.materials;
  const teacher = isTeacher();

  const refreshButton = makeButton('刷新', {
    variant: 'secondary',
    onClick: () => refreshMaterials(),
  });

  const nodes = [
    viewHead(
      '材料',
      teacher ? '上传班级文本材料，系统会切分并建立检索索引。' : '查看老师为班级准备的学习材料（只读）。',
      [refreshButton],
    ),
  ];

  if (teacher) nodes.push(buildUploadCard());
  else {
    nodes.push(h('div', { class: 'notice-card' },
      h('span', { class: 'notice-icon', 'aria-hidden': 'true' }, 'i'),
      h('span', {}, '学生账号为只读：可以浏览材料与原文，不能上传或重建索引。'),
    ));
  }

  nodes.push(buildMaterialsListCard());
  panel.replaceChildren(...nodes);
  await refreshMaterials();
}

function buildUploadCard() {
  const form = h('form', { class: 'upload-form', id: 'upload-form' });

  const fileInput = h('input', {
    type: 'file',
    name: 'file',
    id: 'upload-file',
    accept: '.txt,.md,text/plain,text/markdown',
    required: true,
  });
  const titleInput = h('input', {
    type: 'text', name: 'title', id: 'upload-title', maxlength: 255,
    placeholder: '留空则使用文件名',
  });
  const strategySelect = h('select', { name: 'strategy', id: 'upload-strategy' },
    h('option', { value: 'auto', selected: true }, '自动（推荐）'),
    h('option', { value: 'custom' }, '自定义切分'),
    h('option', { value: 'hierarchy' }, '按层级切分'),
  );
  const maxChars = h('input', {
    type: 'number', name: 'chunk_max_chars', min: 100, max: 4000,
    placeholder: '使用服务器默认值',
  });
  const overlap = h('input', {
    type: 'number', name: 'chunk_overlap', min: 0, max: 1000,
    placeholder: '使用服务器默认值',
  });
  const stripUrls = h('input', { type: 'checkbox', name: 'strip_urls', value: '1' });
  const collapseWs = h('input', { type: 'checkbox', name: 'collapse_whitespace', value: '1' });

  const formError = h('p', { class: 'form-error', role: 'alert', hidden: true });
  const submit = makeButton('上传并索引', { variant: 'primary', type: 'submit' });

  form.append(
    h('div', { class: 'form-grid' },
      h('div', { class: 'field field-wide' },
        h('label', { for: 'upload-file' }, '材料文件'),
        fileInput,
        h('p', { class: 'field-hint' }, '支持 UTF-8 编码的 .txt / .md 文件，大小以服务器配置为准。'),
      ),
      h('div', { class: 'field' },
        h('label', { for: 'upload-title' }, '材料标题（可选）'),
        titleInput,
      ),
      h('div', { class: 'field' },
        h('label', { for: 'upload-strategy' }, '切分策略'),
        strategySelect,
      ),
    ),
    h('details', { class: 'advanced-panel' },
      h('summary', {}, '高级切分选项'),
      h('div', { class: 'form-grid' },
        h('div', { class: 'field' },
          h('label', {}, '最大字符数'),
          maxChars,
        ),
        h('div', { class: 'field' },
          h('label', {}, '切片重叠字符'),
          overlap,
        ),
        h('div', { class: 'field checkbox-field field-wide' },
          h('label', { class: 'check-item' }, stripUrls, h('span', {}, '移除正文中的网址')),
          h('label', { class: 'check-item' }, collapseWs, h('span', {}, '合并多余空白')),
        ),
      ),
    ),
    formError,
    h('div', { class: 'form-actions' }, submit),
  );

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    formError.hidden = true;
    const file = fileInput.files?.[0];
    if (!file) {
      formError.textContent = '请选择一个 .txt 或 .md 文件。';
      formError.hidden = false;
      return;
    }
    const formData = new FormData();
    formData.append('file', file);
    if (titleInput.value.trim()) formData.append('title', titleInput.value.trim());
    formData.append('strategy', strategySelect.value || 'auto');
    if (maxChars.value) formData.append('chunk_max_chars', maxChars.value);
    if (overlap.value) formData.append('chunk_overlap', overlap.value);
    if (stripUrls.checked) formData.append('strip_urls', '1');
    if (collapseWs.checked) formData.append('collapse_whitespace', '1');

    setBusy(submit, true);
    try {
      const result = await api('/materials', { method: 'POST', body: formData });
      const chunks = Number(result?.chunks) || 0;
      showToast(`材料已上传，切分 ${chunks} 段。`, 'success');
      if (result?.index_error) {
        showToast(`材料已保存，但索引未完成：${result.index_error}`, 'error', 8000);
      }
      form.reset();
      await refreshMaterials();
      if (result?.id) openMaterialDrawer(result.id);
    } catch (error) {
      formError.textContent = error?.message || '上传失败，请稍后重试。';
      formError.hidden = false;
      showToast(error?.message || '上传失败。', 'error');
    } finally {
      setBusy(submit, false);
    }
  });

  return h('section', { class: 'card upload-card' },
    h('div', { class: 'card-head' },
      h('div', {},
        h('h2', { class: 'card-title' }, '上传班级材料'),
        h('p', { class: 'card-sub' }, '上传成功后会自动切分、索引，供智能体检索。'),
      ),
      pill('教师功能', 'red'),
    ),
    form,
  );
}

function buildMaterialsListCard() {
  const searchInput = h('input', {
    type: 'search',
    value: state.materialQuery,
    placeholder: '按标题搜索材料…',
    'aria-label': '搜索材料标题',
  });
  const form = h('form', { class: 'search-form' },
    searchInput,
    h('button', { type: 'submit', class: 'btn btn-secondary btn-sm' },
      h('span', { class: 'btn-label' }, '搜索'),
    ),
  );
  form.addEventListener('submit', (event) => {
    event.preventDefault();
    state.materialQuery = searchInput.value.trim();
    refreshMaterials();
  });

  return h('section', { class: 'card materials-card' },
    h('div', { class: 'card-head card-head-wrap' },
      h('div', {},
        h('h2', { class: 'card-title' }, '班级材料'),
        h('p', { class: 'card-sub' }, '点击材料标题查看正文、索引状态与下载入口。'),
      ),
      form,
    ),
    h('div', { id: 'materials-list', class: 'materials-list' }, loadingState('正在加载材料…')),
  );
}

async function refreshMaterials() {
  const host = document.getElementById('materials-list');
  if (!host) return;
  const seq = ++state.materialsSeq;
  host.replaceChildren(loadingState('正在加载材料…'));
  const query = state.materialQuery
    ? `?q=${encodeURIComponent(state.materialQuery)}`
    : '';
  try {
    const data = await api(`/materials${query}`);
    if (seq !== state.materialsSeq) return;
    state.materials = Array.isArray(data?.items) ? data.items : [];
    renderMaterialsTable(host);
  } catch (error) {
    if (seq !== state.materialsSeq) return;
    if (error?.status === 401) return;
    host.replaceChildren(
      inlineError(error?.message || '材料列表加载失败。', () => refreshMaterials()),
    );
  }
}

function renderMaterialsTable(host) {
  if (!state.materials.length) {
    host.replaceChildren(
      emptyState(
        state.materialQuery ? '没有匹配的材料' : '暂无材料',
        state.materialQuery
          ? '尝试更换关键词，或清空搜索。'
          : (isTeacher() ? '请在上方上传第一份班级材料。' : '老师还没有上传班级材料。'),
      ),
    );
    return;
  }

  const tbody = h('tbody');
  state.materials.forEach((material) => {
    const titleButton = h('button', { type: 'button', class: 'link-button' }, material.title || '未命名材料');
    titleButton.addEventListener('click', () => openMaterialDrawer(material.id));
    const viewButton = makeButton('查看', {
      variant: 'ghost',
      className: 'btn-xs',
      onClick: () => openMaterialDrawer(material.id),
    });
    tbody.append(h('tr', {},
      h('td', { class: 'cell-title' }, titleButton),
      h('td', { class: 'cell-muted' }, formatBytes(material.size_bytes)),
      h('td', {}, String(material.chunk_count ?? 0)),
      h('td', {}, indexStatusPill(material.index_status)),
      h('td', { class: 'cell-muted' }, formatTime(material.created_at)),
      h('td', { class: 'cell-actions' }, viewButton),
    ));
  });

  host.replaceChildren(h('div', { class: 'table-wrap' },
    h('table', { class: 'data-table' },
      h('thead', {},
        h('tr', {},
          h('th', {}, '材料'),
          h('th', {}, '大小'),
          h('th', {}, '切片'),
          h('th', {}, '索引'),
          h('th', {}, '上传时间'),
          h('th', { class: 'th-actions' }, '操作'),
        ),
      ),
      tbody,
    ),
  ));
}

function openMaterialDrawer(id) {
  if (state.drawer) closeDrawer();
  const seq = ++state.drawerSeq;

  const closeButton = h('button', { type: 'button', class: 'icon-btn drawer-close', 'aria-label': '关闭材料详情' }, '×');
  closeButton.addEventListener('click', closeDrawer);

  const title = h('h2', { class: 'drawer-title', id: 'drawer-material-title' }, '材料详情');
  const body = h('div', { class: 'drawer-body' }, loadingState('正在加载材料详情…'));
  const drawer = h('aside', {
    class: 'drawer', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'drawer-material-title',
  },
    h('div', { class: 'drawer-head' },
      h('div', {},
        h('p', { class: 'eyebrow' }, '材料详情'),
        title,
      ),
      closeButton,
    ),
    body,
  );
  const overlay = h('div', { class: 'drawer-overlay' }, drawer);
  overlay.addEventListener('click', (event) => {
    if (event.target === overlay) closeDrawer();
  });

  document.body.append(overlay);
  document.body.classList.add('drawer-open');
  state.drawer = { overlay, drawer, id, seq };
  requestAnimationFrame(() => overlay.classList.add('open'));
  closeButton.focus();
  loadMaterialDetail(id, seq);
}

async function loadMaterialDetail(id, seq) {
  if (!state.drawer || state.drawer.seq !== seq) return;
  const body = state.drawer.drawer.querySelector('.drawer-body');
  const title = state.drawer.drawer.querySelector('.drawer-title');
  if (!body) return;
  body.replaceChildren(loadingState('正在加载材料详情…'));
  try {
    const detail = await api(`/materials/${id}`);
    if (!state.drawer || state.drawer.seq !== seq) return;
    if (title) title.textContent = detail.title || '未命名材料';
    renderMaterialDetail(detail, body);
  } catch (error) {
    if (!state.drawer || state.drawer.seq !== seq) return;
    if (error?.status === 401) return;
    body.replaceChildren(
      inlineError(error?.message || '材料详情加载失败。', () => loadMaterialDetail(id, seq)),
    );
  }
}

function renderMaterialDetail(detail, body) {
  const teacher = isTeacher();
  const downloadButton = makeButton('下载原文', {
    variant: 'primary',
    onClick: () => downloadMaterial(detail, downloadButton),
  });

  const actions = h('div', { class: 'drawer-actions' }, downloadButton);
  if (!teacher) {
    actions.append(h('span', { class: 'muted small' }, '学生可下载原文，索引信息为只读。'));
  }

  const meta = h('div', { class: 'meta-grid' },
    metaItem('切片数量', String(detail.chunk_count ?? 0)),
    metaItem('索引状态', indexStatusPill(detail.index_status), true),
    metaItem('文件大小', formatBytes(detail.size_bytes)),
    metaItem('上传时间', formatTime(detail.created_at)),
    metaItem('上传者', `#${detail.uploaded_by ?? '—'}`),
  );

  const bodyText = h('pre', { class: 'material-body' });
  bodyText.textContent = detail.body_text || '';

  const nodes = [
    meta,
    actions,
    h('section', { class: 'drawer-section' },
      h('div', { class: 'section-title-row' },
        h('h3', {}, '材料正文'),
        h('span', { class: 'muted small' }, `${Array.from(detail.body_text || '').length} 字`),
      ),
      detail.body_text ? bodyText : emptyState('暂无正文', '材料正文为空。'),
    ),
  ];

  if (teacher) nodes.push(buildReindexPanel(detail));

  body.replaceChildren(...nodes);
}

function metaItem(label, value, isNode = false) {
  return h('div', { class: 'meta-item' },
    h('span', { class: 'meta-label' }, label),
    isNode && value instanceof Node ? value : h('span', { class: 'meta-value' }, value),
  );
}

function buildReindexPanel(detail) {
  const strategy = h('select', {},
    h('option', { value: 'auto', selected: true }, '自动'),
    h('option', { value: 'custom' }, '自定义'),
    h('option', { value: 'hierarchy' }, '层级'),
  );
  const maxChars = h('input', { type: 'number', min: 100, max: 4000, placeholder: '默认' });
  const overlap = h('input', { type: 'number', min: 0, max: 1000, placeholder: '默认' });
  const error = h('p', { class: 'form-error', role: 'alert', hidden: true });
  const submit = makeButton('重建索引', { variant: 'secondary', type: 'submit' });

  const form = h('form', { class: 'reindex-form' },
    h('div', { class: 'reindex-grid' },
      h('div', { class: 'field' },
        h('label', {}, '策略'),
        strategy,
      ),
      h('div', { class: 'field' },
        h('label', {}, '最大字符数'),
        maxChars,
      ),
      h('div', { class: 'field' },
        h('label', {}, '重叠字符'),
        overlap,
      ),
    ),
    h('p', { class: 'field-hint' }, '重建索引会清空当前切片并重新切分。'),
    error,
    h('div', { class: 'form-actions' }, submit),
  );

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    setBusy(submit, true);
    try {
      const result = await api(`/materials/${detail.id}/reindex`, {
        method: 'POST',
        body: {
          strategy: strategy.value || 'auto',
          chunk_max_chars: Number(maxChars.value) || 0,
          chunk_overlap: Number(overlap.value) || 0,
          strip_urls: false,
          collapse_whitespace: false,
        },
      });
      showToast(`索引已重建：${result?.chunks ?? 0} 个切片（${result?.strategy || 'auto'}）。`, 'success');
      if (state.drawer) await loadMaterialDetail(detail.id, state.drawer.seq);
      await refreshMaterials();
    } catch (err) {
      error.textContent = err?.message || '重建索引失败。';
      error.hidden = false;
      showToast(err?.message || '重建索引失败。', 'error');
    } finally {
      setBusy(submit, false);
    }
  });

  return h('section', { class: 'drawer-section reindex-panel' },
    h('h3', {}, '重建索引'),
    form,
  );
}

async function downloadMaterial(detail, button) {
  setBusy(button, true);
  try {
    const headers = new Headers();
    if (state.token) headers.set('Authorization', `Bearer ${state.token}`);
    const response = await fetch(apiUrl(`/materials/${detail.id}/file`), { headers });
    if (response.status === 401) {
      handleUnauthorized();
      throw new ApiError('登录已过期，请重新登录。', 401, 'unauthorized');
    }
    if (!response.ok) throw await errorFromResponse(response);

    const blob = await response.blob();
    const objectUrl = URL.createObjectURL(blob);
    const contentType = response.headers.get('content-type') || '';
    const titleExtension = String(detail.title || '').toLowerCase().match(/\.(md|txt)$/);
    const extension = contentType.includes('markdown') || detail.mime_type === 'text/markdown' || titleExtension?.[1] === 'md'
      ? '.md'
      : '.txt';
    const safeTitle = String(detail.title || 'material')
      .replace(/[\\/:*?"<>|]/g, '_')
      .replace(/\.(md|txt)$/i, '');
    const link = h('a', { href: objectUrl, download: `${safeTitle}${extension}` });
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(objectUrl), 1500);
    showToast('已开始下载材料原文。', 'success');
  } catch (error) {
    if (error?.status !== 401) showToast(error?.message || '下载失败。', 'error');
  } finally {
    setBusy(button, false);
  }
}

/* --------------------------------------------------------------------------
 * Agents
 * ------------------------------------------------------------------------ */

async function loadAgentsView() {
  const panel = els.panels.agents;
  panel.replaceChildren(loadingState('正在加载智能体…'));
  let agentsPayload;
  let skillsPayload = null;
  let materialsPayload = null;
  try {
    [agentsPayload, skillsPayload, materialsPayload] = await Promise.all([
      api('/agents'),
      api('/skills').catch(() => null),
      api('/materials').catch(() => null),
    ]);
  } catch (error) {
    if (error?.status === 401) return;
    renderViewError(panel, error, () => loadAgentsView());
    return;
  }

  state.agents = Array.isArray(agentsPayload?.items) ? agentsPayload.items : [];
  if (skillsPayload) state.skills = Array.isArray(skillsPayload.items) ? skillsPayload.items : [];
  if (materialsPayload) state.materials = Array.isArray(materialsPayload.items) ? materialsPayload.items : [];

  renderAgentsView();
}

function renderAgentsView() {
  const panel = els.panels.agents;
  const teacher = isTeacher();
  const actions = [
    makeButton('刷新', { variant: 'secondary', onClick: () => loadAgentsView() }),
  ];
  if (teacher) {
    actions.push(makeButton('新建智能体', {
      variant: 'primary', icon: '+', onClick: () => openAgentEditor(null),
    }));
  }

  const nodes = [
    viewHead(
      '智能体',
      teacher ? '配置智能体的提示词、启停状态，以及绑定的技能与材料。' : '查看老师为班级启用的智能体（只读）。',
      actions,
    ),
  ];

  if (!teacher) {
    nodes.push(h('div', { class: 'notice-card' },
      h('span', { class: 'notice-icon', 'aria-hidden': 'true' }, 'i'),
      h('span', {}, '学生账号只能使用已启用的智能体，不能修改配置。'),
    ));
  }

  if (!state.agents.length) {
    nodes.push(h('div', { class: 'card' },
      emptyState(
        teacher ? '还没有智能体' : '暂无可用智能体',
        teacher ? '创建一个智能体，绑定技能与材料后即可在对话中使用。' : '请联系老师启用智能体。',
        teacher ? makeButton('新建智能体', { variant: 'primary', onClick: () => openAgentEditor(null) }) : null,
      ),
    ));
  } else {
    const grid = h('div', { class: 'agent-grid' });
    state.agents.forEach((agent) => grid.append(buildAgentCard(agent, teacher)));
    nodes.push(grid);
  }

  panel.replaceChildren(...nodes);
}

function buildAgentCard(agent, teacher) {
  const enabled = agent.enabled !== false;
  const skillIds = Array.isArray(agent.skill_ids) ? agent.skill_ids : [];
  const materialIds = Array.isArray(agent.material_ids) ? agent.material_ids : [];
  const headActions = [];
  if (teacher) {
    headActions.push(makeButton('编辑', {
      variant: 'ghost', className: 'btn-xs', onClick: () => openAgentEditor(agent),
    }));
    headActions.push(makeButton('删除', {
      variant: 'danger-ghost', className: 'btn-xs', onClick: () => deleteAgent(agent),
    }));
  }

  const chips = h('div', { class: 'chip-row' });
  if (skillIds.length) {
    skillIds.forEach((id) => chips.append(h('span', { class: 'chip' }, skillNameById(id))));
  } else {
    chips.append(h('span', { class: 'chip chip-empty' }, '未绑定技能'));
  }

  const materialChips = h('div', { class: 'chip-row' });
  if (materialIds.length) {
    materialIds.forEach((id) => materialChips.append(h('span', { class: 'chip chip-material' }, materialTitleById(id))));
  } else {
    materialChips.append(h('span', { class: 'chip chip-empty' }, '未绑定材料'));
  }

  return h('article', { class: `agent-card${enabled ? '' : ' is-disabled'}` },
    h('div', { class: 'agent-head' },
      h('div', { class: 'agent-title-wrap' },
        h('h2', { class: 'agent-name' }, agent.name || '未命名智能体'),
        enabled ? pill('已启用', 'green') : pill('已停用', 'gray'),
      ),
      teacher ? h('div', { class: 'card-actions' }, ...headActions) : null,
    ),
    h('p', { class: 'agent-prompt' }, agent.prompt || '未填写提示词。'),
    h('div', { class: 'agent-bindings' },
      h('div', { class: 'binding-row' },
        h('span', { class: 'binding-label' }, '技能'),
        chips,
      ),
      h('div', { class: 'binding-row' },
        h('span', { class: 'binding-label' }, '材料'),
        materialChips,
      ),
    ),
  );
}

async function openAgentEditor(agent) {
  if (!isTeacher()) return;
  const editing = Boolean(agent);
  if (!state.skills.length || !state.materials.length) {
    try {
      const [skills, materials] = await Promise.all([
        api('/skills').catch(() => null),
        api('/materials').catch(() => null),
      ]);
      if (skills) state.skills = Array.isArray(skills.items) ? skills.items : [];
      if (materials) state.materials = Array.isArray(materials.items) ? materials.items : [];
    } catch {
      /* keep existing caches */
    }
  }

  const nameInput = h('input', {
    type: 'text', name: 'name', maxlength: 64, value: agent?.name || '',
    placeholder: '例如：数学学习助手', required: true,
  });
  const promptInput = h('textarea', {
    name: 'prompt', rows: 5, maxlength: 2000, value: agent?.prompt || '',
    placeholder: '描述这个智能体如何帮助学生…',
  });
  const enabledInput = h('input', {
    type: 'checkbox', name: 'enabled', checked: agent ? agent.enabled !== false : true,
  });
  const selectedSkillIds = new Set((agent?.skill_ids || []).map(String));
  const selectedMaterialIds = new Set((agent?.material_ids || []).map(String));

  const skillChecks = h('div', { class: 'check-grid' });
  if (state.skills.length) {
    state.skills.forEach((skill) => {
      const input = h('input', {
        type: 'checkbox', value: String(skill.id), checked: selectedSkillIds.has(String(skill.id)),
      });
      skillChecks.append(h('label', { class: 'check-item' },
        input,
        h('span', {}, skill.name || '未命名技能'),
      ));
    });
  } else {
    skillChecks.append(h('p', { class: 'field-hint' }, '暂无可用技能。'));
  }

  const materialChecks = h('div', { class: 'check-grid check-grid-tall' });
  if (state.materials.length) {
    state.materials.forEach((material) => {
      const input = h('input', {
        type: 'checkbox', value: String(material.id), checked: selectedMaterialIds.has(String(material.id)),
      });
      materialChecks.append(h('label', { class: 'check-item' },
        input,
        h('span', {}, material.title || '未命名材料'),
      ));
    });
  } else {
    materialChecks.append(h('p', { class: 'field-hint' }, '暂无可用材料。'));
  }

  const error = h('p', { class: 'form-error', role: 'alert', hidden: true });
  const form = h('form', { id: 'agent-form', class: 'dialog-form' },
    h('div', { class: 'form-grid' },
      h('div', { class: 'field field-wide' },
        h('label', {}, '名称'),
        nameInput,
        h('p', { class: 'field-hint' }, '班内名称必须唯一，最多 64 字。'),
      ),
      h('div', { class: 'field field-wide' },
        h('label', {}, '提示词'),
        promptInput,
        h('p', { class: 'field-hint' }, '最多 2000 字。'),
      ),
      h('div', { class: 'field field-wide checkbox-field' },
        h('label', { class: 'check-item' }, enabledInput, h('span', {}, '启用该智能体（学生可见）')),
      ),
      h('div', { class: 'field field-wide' },
        h('label', {}, '绑定技能'),
        skillChecks,
      ),
      h('div', { class: 'field field-wide' },
        h('label', {}, '绑定材料'),
        materialChecks,
      ),
    ),
    error,
  );

  const submit = makeButton(editing ? '保存修改' : '创建智能体', {
    variant: 'primary', type: 'submit',
  });
  submit.setAttribute('form', 'agent-form');
  const cancel = makeButton('取消', { variant: 'secondary', onClick: closeModal });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    const name = nameInput.value.trim();
    if (!name) {
      error.textContent = '请填写智能体名称。';
      error.hidden = false;
      return;
    }
    const payload = {
      name,
      prompt: promptInput.value,
      enabled: enabledInput.checked,
      skill_ids: Array.from(skillChecks.querySelectorAll('input:checked')).map((input) => Number(input.value)),
      material_ids: Array.from(materialChecks.querySelectorAll('input:checked')).map((input) => Number(input.value)),
    };
    setBusy(submit, true);
    try {
      if (editing) await api(`/agents/${agent.id}`, { method: 'PUT', body: payload });
      else await api('/agents', { method: 'POST', body: payload });
      closeModal();
      showToast(editing ? '智能体已更新。' : '智能体已创建。', 'success');
      await loadAgentsView();
    } catch (err) {
      error.textContent = err?.message || '保存智能体失败。';
      error.hidden = false;
      showToast(err?.message || '保存智能体失败。', 'error');
    } finally {
      setBusy(submit, false);
    }
  });

  openModal({
    title: editing ? '编辑智能体' : '新建智能体',
    body: form,
    footer: h('div', { class: 'modal-actions' }, cancel, submit),
    wide: true,
  });
}

async function deleteAgent(agent) {
  const confirmed = await confirmDialog({
    title: '删除智能体',
    message: `确定删除“${agent.name || '未命名智能体'}”吗？如果已有会话使用该智能体，服务器会拒绝删除。`,
    confirmText: '删除',
    danger: true,
  });
  if (!confirmed) return;
  try {
    await api(`/agents/${agent.id}`, { method: 'DELETE' });
    showToast('智能体已删除。', 'success');
    await loadAgentsView();
  } catch (error) {
    showToast(error?.message || '删除智能体失败。', 'error');
  }
}

/* --------------------------------------------------------------------------
 * Prompt & skills
 * ------------------------------------------------------------------------ */

async function loadSkillsView() {
  const panel = els.panels.skills;
  panel.replaceChildren(loadingState('正在加载提示词与技能…'));
  let promptPayload;
  let skillsPayload;
  try {
    [promptPayload, skillsPayload] = await Promise.all([
      api('/prompt'),
      api('/skills'),
    ]);
  } catch (error) {
    if (error?.status === 401) return;
    renderViewError(panel, error, () => loadSkillsView());
    return;
  }
  state.skills = Array.isArray(skillsPayload?.items) ? skillsPayload.items : [];
  renderSkillsView(promptPayload?.prompt || '');
}

function renderSkillsView(prompt) {
  const panel = els.panels.skills;
  const teacher = isTeacher();
  const refreshButton = makeButton('刷新', {
    variant: 'secondary', onClick: () => loadSkillsView(),
  });
  const newButton = teacher ? makeButton('新建技能', {
    variant: 'primary', icon: '+', onClick: () => openSkillEditor(null),
  }) : null;

  const promptCard = buildPromptCard(prompt, teacher);
  const skillsHead = h('div', { class: 'section-head' },
    h('div', {},
      h('h2', { class: 'section-title' }, '班级技能'),
      h('p', { class: 'section-sub' }, teacher ? '技能文件由元数据与引导正文组成，保存前会校验工具引用。' : '以下技能由老师配置，学生只读。'),
    ),
    h('div', { class: 'section-actions' }, newButton),
  );

  let skillsContent;
  if (!state.skills.length) {
    skillsContent = h('div', { class: 'card' },
      emptyState(
        teacher ? '还没有技能' : '暂无技能',
        teacher ? '创建技能后，把它绑定到智能体即可在对话中生效。' : '老师还没有为班级配置技能。',
        teacher ? makeButton('新建技能', { variant: 'primary', onClick: () => openSkillEditor(null) }) : null,
      ),
    );
  } else {
    const grid = h('div', { class: 'skill-grid' });
    state.skills.forEach((skill) => grid.append(buildSkillCard(skill, teacher)));
    skillsContent = grid;
  }

  panel.replaceChildren(
    viewHead('提示词与技能', '设置班级默认提示词，并维护可复用的引导技能。', [refreshButton]),
    promptCard,
    skillsHead,
    skillsContent,
  );
}

function buildPromptCard(prompt, teacher) {
  const textarea = h('textarea', {
    id: 'class-prompt',
    rows: 7,
    maxlength: 2000,
    value: prompt,
    readOnly: !teacher,
    placeholder: '输入班级默认提示词…',
  });
  const count = h('span', { class: 'char-count' }, `${Array.from(prompt).length}/2000`);
  const saveButton = teacher ? makeButton('保存提示词', { variant: 'primary', type: 'button' }) : null;
  const error = h('p', { class: 'form-error', role: 'alert', hidden: true });

  textarea.addEventListener('input', () => {
    count.textContent = `${Array.from(textarea.value).length}/2000`;
  });

  saveButton?.addEventListener('click', async () => {
    error.hidden = true;
    setBusy(saveButton, true);
    try {
      await api('/prompt', { method: 'PUT', body: { prompt: textarea.value } });
      showToast('班级提示词已保存。', 'success');
    } catch (err) {
      error.textContent = err?.message || '保存提示词失败。';
      error.hidden = false;
      showToast(err?.message || '保存提示词失败。', 'error');
    } finally {
      setBusy(saveButton, false);
    }
  });

  return h('section', { class: 'card prompt-card' },
    h('div', { class: 'card-head' },
      h('div', {},
        h('h2', { class: 'card-title' }, '班级默认提示词'),
        h('p', { class: 'card-sub' }, teacher ? '应用于本班所有智能体对话。最多 2000 字。' : '由老师维护，当前为只读。'),
      ),
      teacher ? pill('可编辑', 'red') : pill('只读', 'gray'),
    ),
    textarea,
    error,
    h('div', { class: 'card-foot' },
      count,
      saveButton,
    ),
  );
}

function buildSkillCard(skill, teacher) {
  const enabled = skill.enabled !== false;
  const tools = Array.isArray(skill.tools) ? skill.tools : [];
  const actions = [];
  if (teacher) {
    actions.push(makeButton('编辑', {
      variant: 'ghost', className: 'btn-xs', onClick: () => openSkillEditor(skill),
    }));
    actions.push(makeButton('删除', {
      variant: 'danger-ghost', className: 'btn-xs', onClick: () => deleteSkill(skill),
    }));
  }

  const trigger = h('span', { class: 'skill-trigger' }, skill.trigger_text || '未设置适用条件');

  return h('article', { class: `skill-card${enabled ? '' : ' is-disabled'}` },
    h('div', { class: 'skill-head' },
      h('div', { class: 'skill-title-wrap' },
        h('h3', { class: 'skill-name' }, skill.name || '未命名技能'),
        enabled ? pill('已启用', 'green') : pill('已停用', 'gray'),
      ),
      teacher ? h('div', { class: 'card-actions' }, ...actions) : null,
    ),
    h('p', { class: 'skill-desc' }, skill.description || '未填写引导正文。'),
    h('div', { class: 'skill-meta' },
      h('span', { class: 'meta-label' }, '适用'),
      trigger,
      skill.subject_text ? h('span', { class: 'chip chip-subject' }, skill.subject_text) : null,
    ),
    h('div', { class: 'tool-row' },
      h('span', { class: 'meta-label' }, '工具'),
      tools.length
        ? tools.map((tool) => h('code', { class: 'tool-chip' }, tool))
        : h('span', { class: 'chip chip-empty' }, '未配置工具'),
    ),
  );
}

function openSkillEditor(skill) {
  if (!isTeacher()) return;
  const editing = Boolean(skill);
  const initial = skill?.source_text || buildSkillSourceFallback(skill);
  const textarea = h('textarea', {
    id: 'skill-source',
    class: 'code-textarea',
    rows: 18,
    value: initial,
    spellcheck: false,
    placeholder: '名称：…\n适用条件：…\n资料：…\n工具：knowledge_search\n启用：是\n\n（空行以下为技能引导正文）',
  });
  const loadSample = makeButton('载入示例', { variant: 'ghost', type: 'button' });
  loadSample.addEventListener('click', () => {
    textarea.value = SKILL_SAMPLE;
    textarea.focus();
    textarea.setSelectionRange(0, 0);
    textarea.scrollTop = 0;
  });

  const formatHint = h('div', { class: 'skill-format-hint' },
    h('span', {}, '格式：元数据行 + 空行 + 引导正文。'),
    h('span', { class: 'muted' }, '工具仅支持 knowledge_search 或 工具名@服务器名（须先连接 MCP）。'),
  );

  const error = h('p', { class: 'form-error', role: 'alert', hidden: true });
  const form = h('form', { id: 'skill-form', class: 'dialog-form' },
    h('div', { class: 'field' },
      h('div', { class: 'field-label-row' },
        h('label', { for: 'skill-source' }, '完整技能文件（source_text）'),
        loadSample,
      ),
      textarea,
      formatHint,
    ),
    error,
  );

  const submit = makeButton(editing ? '保存修改' : '创建技能', {
    variant: 'primary', type: 'submit',
  });
  submit.setAttribute('form', 'skill-form');
  const cancel = makeButton('取消', { variant: 'secondary', onClick: closeModal });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    const sourceText = textarea.value;
    if (!sourceText.trim()) {
      error.textContent = '技能文件不能为空。';
      error.hidden = false;
      return;
    }
    setBusy(submit, true);
    try {
      if (editing) await api(`/skills/${skill.id}`, { method: 'PUT', body: { source_text: sourceText } });
      else await api('/skills', { method: 'POST', body: { source_text: sourceText } });
      closeModal();
      showToast(editing ? '技能已更新。' : '技能已创建。', 'success');
      await loadSkillsView();
    } catch (err) {
      error.textContent = err?.message || '保存技能失败。';
      error.hidden = false;
      showToast(err?.message || '保存技能失败。', 'error');
    } finally {
      setBusy(submit, false);
    }
  });

  openModal({
    title: editing ? '编辑技能' : '新建技能',
    body: form,
    footer: h('div', { class: 'modal-actions' }, cancel, submit),
    wide: true,
  });
}

function buildSkillSourceFallback(skill) {
  if (!skill) return '';
  const tools = Array.isArray(skill.tools) && skill.tools.length ? skill.tools.join(', ') : 'knowledge_search';
  return [
    `名称：${skill.name || ''}`,
    `适用条件：${skill.trigger_text || ''}`,
    `资料：${skill.subject_text || ''}`,
    `工具：${tools}`,
    `启用：${skill.enabled === false ? '否' : '是'}`,
    '',
    skill.description || '',
  ].join('\n');
}

async function deleteSkill(skill) {
  const confirmed = await confirmDialog({
    title: '删除技能',
    message: `确定删除“${skill.name || '未命名技能'}”吗？已绑定它的智能体将失去该技能。`,
    confirmText: '删除',
    danger: true,
  });
  if (!confirmed) return;
  try {
    await api(`/skills/${skill.id}`, { method: 'DELETE' });
    showToast('技能已删除。', 'success');
    await loadSkillsView();
  } catch (error) {
    showToast(error?.message || '删除技能失败。', 'error');
  }
}

/* --------------------------------------------------------------------------
 * MCP servers
 * ------------------------------------------------------------------------ */

async function loadMcpView() {
  if (!isTeacher()) { navigate('chat'); return; }
  const panel = els.panels.mcp;
  panel.replaceChildren(loadingState('正在加载 MCP 服务器…'));
  try {
    const data = await api('/mcp/servers');
    state.mcpServers = Array.isArray(data?.items) ? data.items : [];
    renderMcpView();
  } catch (error) {
    if (error?.status === 401) return;
    renderViewError(panel, error, () => loadMcpView());
  }
}

function renderMcpView() {
  const panel = els.panels.mcp;
  const refreshButton = makeButton('刷新', {
    variant: 'secondary', onClick: () => loadMcpView(),
  });

  const nodes = [
    viewHead('MCP 工具', '连接班级 MCP 服务器，发现工具并供技能引用。', [refreshButton]),
    buildMcpRegisterCard(),
  ];

  if (!state.mcpServers.length) {
    nodes.push(h('div', { class: 'card' },
      emptyState('还没有 MCP 服务器', '注册并连接服务器后，技能中即可引用其工具。'),
    ));
  } else {
    const grid = h('div', { class: 'server-grid' });
    state.mcpServers.forEach((server) => grid.append(buildServerCard(server)));
    nodes.push(grid);
  }

  panel.replaceChildren(...nodes);
}

function buildMcpRegisterCard() {
  const name = h('input', {
    type: 'text', name: 'name', id: 'mcp-name', value: 'campus-tools',
    pattern: '[a-z][a-z0-9_\\-]{0,63}', maxlength: 64, placeholder: 'campus-tools', required: true,
  });
  const baseUrl = h('input', {
    type: 'url', name: 'base_url', id: 'mcp-base-url', value: 'http://mcp-demo:8090',
    maxlength: 512, placeholder: 'http://mcp-demo:8090', required: true,
  });
  const token = h('input', {
    type: 'password', name: 'auth_token', id: 'mcp-token',
    autocomplete: 'new-password', placeholder: '可选，不会回显',
  });
  const error = h('p', { class: 'form-error', role: 'alert', hidden: true });
  const submit = makeButton('注册服务器', { variant: 'primary', type: 'submit' });

  const form = h('form', { class: 'mcp-form' },
    h('div', { class: 'form-grid' },
      h('div', { class: 'field' },
        h('label', { for: 'mcp-name' }, '服务器名称'),
        name,
        h('p', { class: 'field-hint' }, '小写字母开头，仅含小写字母、数字、下划线、连字符。'),
      ),
      h('div', { class: 'field' },
        h('label', { for: 'mcp-base-url' }, 'Base URL'),
        baseUrl,
      ),
      h('div', { class: 'field field-wide' },
        h('label', { for: 'mcp-token' }, '认证令牌（可选）'),
        token,
        h('p', { class: 'field-hint' }, '令牌仅用于连接，保存后不会回显。'),
      ),
    ),
    error,
    h('div', { class: 'form-actions' }, submit),
  );

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    setBusy(submit, true);
    try {
      await api('/mcp/servers', {
        method: 'POST',
        body: {
          name: name.value.trim(),
          base_url: baseUrl.value.trim(),
          auth_token: token.value.trim(),
        },
      });
      showToast('MCP 服务器已注册。请点击“连接”发现工具。', 'success');
      await loadMcpView();
    } catch (err) {
      error.textContent = err?.message || '注册服务器失败。';
      error.hidden = false;
      showToast(err?.message || '注册服务器失败。', 'error');
    } finally {
      setBusy(submit, false);
    }
  });

  return h('section', { class: 'card' },
    h('div', { class: 'card-head' },
      h('div', {},
        h('h2', { class: 'card-title' }, '注册 MCP 服务器'),
        h('p', { class: 'card-sub' }, '演示环境已预填 campus-tools。'),
      ),
      pill('教师功能', 'red'),
    ),
    form,
  );
}

function buildServerCard(server) {
  const connected = server.connected === true;
  const toolsPanel = h('div', { class: 'tools-panel', hidden: true });
  const toolsButton = makeButton(`查看工具 (${server.tool_count ?? 0})`, {
    variant: 'ghost',
    className: 'btn-xs',
  });
  toolsButton.addEventListener('click', () => toggleMcpTools(server, toolsPanel, toolsButton));

  const connectButton = makeButton(connected ? '重新连接' : '连接', {
    variant: 'secondary', className: 'btn-xs',
    onClick: () => connectMcpServer(server, connectButton),
  });
  const disconnectButton = connected ? makeButton('断开', {
    variant: 'ghost', className: 'btn-xs',
    onClick: () => disconnectMcpServer(server),
  }) : null;
  const editButton = makeButton('编辑', {
    variant: 'ghost', className: 'btn-xs',
    onClick: () => openMcpEdit(server),
  });
  const deleteButton = makeButton('删除', {
    variant: 'danger-ghost', className: 'btn-xs',
    onClick: () => deleteMcpServer(server),
  });

  return h('article', { class: 'server-card' },
    h('div', { class: 'server-head' },
      h('div', { class: 'server-title-wrap' },
        h('h2', { class: 'server-name' }, server.name || '未命名服务器'),
        connected ? pill('已连接', 'green') : pill('未连接', 'gray'),
      ),
      h('code', { class: 'server-url' }, server.base_url || '—'),
    ),
    h('div', { class: 'server-meta' },
      h('span', { class: 'server-meta-item' },
        h('span', { class: 'meta-label' }, '认证令牌'),
        server.has_auth ? pill('已配置', 'blue') : pill('未配置', 'gray'),
      ),
      h('span', { class: 'server-meta-item' },
        h('span', { class: 'meta-label' }, '工具数量'),
        h('strong', {}, String(server.tool_count ?? 0)),
      ),
      server.connected_at
        ? h('span', { class: 'server-meta-item' },
          h('span', { class: 'meta-label' }, '连接时间'),
          h('span', {}, formatTime(server.connected_at)),
        )
        : null,
    ),
    h('div', { class: 'server-actions' },
      connectButton,
      disconnectButton,
      toolsButton,
      editButton,
      deleteButton,
    ),
    toolsPanel,
  );
}

function openMcpEdit(server) {
  const name = h('input', {
    type: 'text', id: 'mcp-edit-name', value: server.name || '', maxlength: 64, required: true,
  });
  const baseUrl = h('input', {
    type: 'url', id: 'mcp-edit-url', value: server.base_url || '', maxlength: 512, required: true,
  });
  const token = h('input', {
    type: 'password', id: 'mcp-edit-token', autocomplete: 'new-password',
    placeholder: '留空表示不修改令牌',
  });
  const error = h('p', { class: 'form-error', role: 'alert', hidden: true });
  const form = h('form', { class: 'dialog-form', id: 'mcp-edit-form' },
    h('div', { class: 'form-grid' },
      h('div', { class: 'field field-wide' },
        h('label', { for: 'mcp-edit-name' }, '服务器名称'), name,
        h('p', { class: 'field-hint' }, '小写字母开头，仅含小写字母、数字、下划线、连字符。'),
      ),
      h('div', { class: 'field field-wide' },
        h('label', { for: 'mcp-edit-url' }, 'Base URL'), baseUrl,
      ),
      h('div', { class: 'field field-wide' },
        h('label', { for: 'mcp-edit-token' }, '认证令牌（可选）'), token,
        h('p', { class: 'field-hint' }, '留空保留原令牌；修改地址或令牌后需要重新连接。'),
      ),
    ),
    error,
  );
  const submit = makeButton('保存修改', { variant: 'primary', type: 'submit' });
  submit.setAttribute('form', 'mcp-edit-form');
  const cancel = makeButton('取消', { variant: 'secondary', onClick: closeModal });
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    error.hidden = true;
    const payload = { name: name.value.trim(), base_url: baseUrl.value.trim() };
    if (token.value.trim()) payload.auth_token = token.value.trim();
    setBusy(submit, true);
    try {
      await api(`/mcp/servers/${server.id}`, { method: 'PUT', body: payload });
      closeModal();
      showToast('服务器已修改，请重新连接以发现工具。', 'success');
      await loadMcpView();
    } catch (err) {
      error.textContent = err?.message || '保存修改失败。';
      error.hidden = false;
      showToast(err?.message || '保存修改失败。', 'error');
    } finally {
      setBusy(submit, false);
    }
  });
  openModal({
    title: `编辑服务器 · ${server.name}`,
    body: form,
    footer: h('div', { class: 'modal-actions' }, cancel, submit),
  });
}

async function toggleMcpTools(server, panel, button) {
  const isOpen = !panel.hidden;
  if (isOpen) {
    panel.hidden = true;
    button.querySelector('.btn-label').textContent = `查看工具 (${server.tool_count ?? 0})`;
    return;
  }
  panel.hidden = false;
  button.querySelector('.btn-label').textContent = '收起工具';
  if (panel.dataset.loaded === 'true') return;

  panel.replaceChildren(loadingState('正在加载工具清单…'));
  try {
    const data = await api(`/mcp/servers/${server.id}/tools`);
    const tools = Array.isArray(data?.tools) ? data.tools : [];
    panel.dataset.loaded = 'true';
    if (!tools.length) {
      panel.replaceChildren(h('p', { class: 'field-hint' }, '该服务器尚未发现任何工具。请先连接。'));
      return;
    }
    const list = h('div', { class: 'tool-list' });
    tools.forEach((tool) => {
      let schema = tool.input_schema || '';
      try { schema = JSON.stringify(JSON.parse(schema), null, 2); } catch { /* keep raw */ }
      list.append(h('div', { class: 'tool-item' },
        h('div', { class: 'tool-item-head' },
          h('code', { class: 'tool-name' }, tool.name || '未命名工具'),
          h('span', { class: 'tool-server' }, server.name || ''),
        ),
        h('p', { class: 'tool-description' }, tool.description || '无描述'),
        schema
          ? h('details', { class: 'schema-details' },
            h('summary', {}, '输入参数 Schema'),
            h('pre', { class: 'schema-block' }, schema),
          )
          : null,
      ));
    });
    panel.replaceChildren(list);
  } catch (error) {
    if (error?.status === 401) return;
    panel.replaceChildren(inlineError(error?.message || '工具清单加载失败。', () => {
      panel.dataset.loaded = 'false';
      panel.hidden = true;
      button.querySelector('.btn-label').textContent = `查看工具 (${server.tool_count ?? 0})`;
      toggleMcpTools(server, panel, button);
    }));
  }
}

async function connectMcpServer(server, button) {
  setBusy(button, true);
  try {
    const result = await api(`/mcp/servers/${server.id}/connect`, { method: 'POST' });
    const count = Array.isArray(result?.tools) ? result.tools.length : 0;
    showToast(`已连接 ${server.name}，发现 ${count} 个工具。`, 'success');
    await loadMcpView();
  } catch (error) {
    if (error?.status !== 401) showToast(error?.message || '连接服务器失败。', 'error');
  } finally {
    setBusy(button, false);
  }
}

async function disconnectMcpServer(server) {
  try {
    await api(`/mcp/servers/${server.id}/disconnect`, { method: 'POST' });
    showToast(`已断开 ${server.name}。`, 'success');
    await loadMcpView();
  } catch (error) {
    if (error?.status !== 401) showToast(error?.message || '断开服务器失败。', 'error');
  }
}

async function deleteMcpServer(server) {
  const confirmed = await confirmDialog({
    title: '删除 MCP 服务器',
    message: `确定删除“${server.name}”吗？如果已有工具调用审计记录，服务器将按设计拒绝删除。`,
    confirmText: '删除',
    danger: true,
  });
  if (!confirmed) return;
  try {
    await api(`/mcp/servers/${server.id}`, { method: 'DELETE' });
    showToast('MCP 服务器已删除。', 'success');
    await loadMcpView();
  } catch (error) {
    showToast(error?.message || '删除服务器失败。', 'error');
  }
}

/* --------------------------------------------------------------------------
 * Tool-call audit
 * ------------------------------------------------------------------------ */

async function loadCallsView() {
  if (!isTeacher()) { navigate('chat'); return; }
  const panel = els.panels.calls;
  panel.replaceChildren(viewHead('调用记录', '查看本班最近 50 条 MCP 工具调用审计。', [
    makeButton('刷新', { variant: 'secondary', onClick: () => loadCallsView() }),
  ]), loadingState('正在加载调用记录…'));

  try {
    const data = await api('/mcp/tool-calls?limit=50');
    const items = Array.isArray(data?.items) ? data.items : [];
    const head = viewHead('调用记录', '查看本班最近 50 条 MCP 工具调用审计。', [
      makeButton('刷新', { variant: 'secondary', onClick: () => loadCallsView() }),
    ]);

    if (!items.length) {
      panel.replaceChildren(head, h('div', { class: 'card' },
        emptyState('暂无调用记录', '当智能体使用 MCP 工具时，调用明细会记录在这里。'),
      ));
      return;
    }

    const tbody = h('tbody');
    items.forEach((call) => {
      tbody.append(h('tr', {},
        h('td', { class: 'cell-muted nowrap' }, formatTime(call.created_at)),
        h('td', {}, h('code', { class: 'tool-ref' }, call.tool_ref || '—')),
        h('td', {}, statusPill(call.status)),
        h('td', {}, call.username || (call.user_id ? `#${call.user_id}` : '—')),
        h('td', { class: 'nowrap' },
          call.conversation_id ? `#${call.conversation_id}` : '—',
        ),
        h('td', { class: 'cell-detail' }, call.detail || '—'),
      ));
    });

    panel.replaceChildren(
      head,
      h('div', { class: 'card' },
        h('div', { class: 'table-wrap' },
          h('table', { class: 'data-table calls-table' },
            h('thead', {},
              h('tr', {},
                h('th', {}, '时间'),
                h('th', {}, '工具引用'),
                h('th', {}, '状态'),
                h('th', {}, '用户'),
                h('th', {}, '会话'),
                h('th', {}, '详情'),
              ),
            ),
            tbody,
          ),
        ),
      ),
    );
  } catch (error) {
    if (error?.status === 401) return;
    renderViewError(panel, error, () => loadCallsView());
  }
}

/* --------------------------------------------------------------------------
 * Start
 * ------------------------------------------------------------------------ */

init();

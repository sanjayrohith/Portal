const state = { requests: new Map(), selected: null };

const listElement = document.querySelector('#request-list');
const countElement = document.querySelector('#request-count');
const capturedElement = document.querySelector('#stat-captured');
const methodElement = document.querySelector('#stat-method');
const statusElement = document.querySelector('#stat-status');
const connectionLabel = document.querySelector('#connection-label');

function statusClass(status) {
  if (status >= 500) return 'error';
  if (status >= 400) return 'warn';
  return 'ok';
}

function renderRequests() {
  const requests = [...state.requests.values()].reverse();
  countElement.textContent = `${requests.length} request${requests.length === 1 ? '' : 's'}`;
  capturedElement.textContent = requests.length;
  if (requests.length === 0) return;

  listElement.innerHTML = requests.map((request) => {
    const status = request.status_code || 0;
    return `<button class="request-row ${state.selected === request.id ? 'selected' : ''}" data-id="${escapeHTML(request.id)}">
      <span class="method">${escapeHTML(request.method || '—')}</span>
      <span class="request-path">${escapeHTML(request.path || '/')}</span>
      <span class="response-code ${statusClass(status)}">${status || '...'}</span>
    </button>`;
  }).join('');
  listElement.querySelectorAll('.request-row').forEach((row) => {
    row.addEventListener('click', () => selectRequest(row.dataset.id));
  });

  const last = requests[0];
  methodElement.textContent = last.method || 'Waiting';
  statusElement.textContent = last.status_code || 'Waiting';
}

function addRequest(request) {
  const summary = request.request ? {
    id: request.id,
    method: request.request.method,
    path: request.request.path || request.request.url,
    status_code: request.response && request.response.status_code,
    started_at: request.started_at,
    duration: request.duration,
    client_ip: request.client_ip || request.request.client_ip
  } : request;
  state.requests.set(summary.id, summary);
  renderRequests();
}

async function loadRequests() {
  try {
    const response = await fetch('/api/requests');
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    (await response.json()).forEach(addRequest);
  } catch (error) {
    connectionLabel.textContent = 'API unavailable';
  }
}

function connectEvents() {
  const events = new EventSource('/api/events');
  events.addEventListener('open', () => { connectionLabel.textContent = 'Live capture'; });
  events.addEventListener('request', (event) => addRequest(JSON.parse(event.data)));
  events.addEventListener('error', () => { connectionLabel.textContent = 'Reconnecting'; });
}

function selectRequest(id) {
  state.selected = id;
  renderRequests();
  fetch(`/api/requests/${encodeURIComponent(id)}`)
    .then((response) => response.json())
    .then(renderDetail)
    .catch(() => {});
}

function renderDetail(transaction) {
  const panel = document.querySelector('#detail-panel');
  const requestURL = transaction.request.url || transaction.request.path || '/';
  let query = '';
  try { query = new URL(requestURL, window.location.origin).search; } catch (_) {}
  const status = transaction.response && transaction.response.status_code;
  panel.innerHTML = `<div class="detail-content">
    <div class="detail-header">
      <div><span class="method detail-method">${escapeHTML(transaction.request.method || 'Request')}</span><h2>${escapeHTML(transaction.request.path || requestURL)}</h2></div>
      <button class="replay-button" id="replay-button" data-id="${escapeHTML(transaction.id)}">Replay</button>
    </div>
    <div class="detail-meta"><span class="response-pill ${statusClass(status || 0)}">${status || 'No response'}</span><span>${formatDuration(transaction.duration)}</span><span>${escapeHTML(transaction.client_ip || transaction.request.client_ip || 'Local')}</span></div>
    ${query ? `<section class="detail-section"><h3>Query parameters</h3><code class="code-block">${escapeHTML(query.slice(1))}</code></section>` : ''}
    <section class="detail-section"><h3>Request headers</h3>${renderHeaders(transaction.request.headers)}</section>
    <section class="detail-section"><h3>Request body</h3>${renderBody(transaction.request.body)}</section>
    ${transaction.response ? `<section class="detail-section response-section"><h3>Response headers</h3>${renderHeaders(transaction.response.headers)}</section><section class="detail-section"><h3>Response body</h3>${renderBody(transaction.response.body)}</section>` : ''}
  </div>`;
  document.querySelector('#replay-button').addEventListener('click', replayRequest);
}

async function replayRequest(event) {
  const button = event.currentTarget;
  button.disabled = true;
  button.textContent = 'Replaying';
  try {
    const response = await fetch(`/api/requests/${encodeURIComponent(button.dataset.id)}/replay`, { method: 'POST' });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || `HTTP ${response.status}`);
    showToast(`Replay returned ${result.status_code}`, 'success');
  } catch (error) {
    showToast(`Replay failed: ${error.message}`, 'error');
  } finally {
    button.disabled = false;
    button.textContent = 'Replay';
  }
}

function showToast(message, type) {
  const existing = document.querySelector('.toast');
  if (existing) existing.remove();
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.setAttribute('role', 'status');
  toast.textContent = message;
  document.body.appendChild(toast);
  window.setTimeout(() => toast.remove(), 3600);
}

function renderHeaders(headers) {
  const entries = Object.entries(headers || {});
  if (entries.length === 0) return '<p class="muted">No headers captured.</p>';
  return `<dl class="headers">${entries.map(([key, values]) => `<div><dt>${escapeHTML(key)}</dt><dd>${escapeHTML(values.join(', '))}</dd></div>`).join('')}</dl>`;
}

function renderBody(body) {
  if (!body || body.length === 0) return '<p class="muted">Empty body.</p>';
  const text = decodeBody(body);
  let formatted = text;
  try { formatted = JSON.stringify(JSON.parse(text), null, 2); } catch (_) {}
  return `<pre class="code-block body-block">${escapeHTML(formatted)}</pre>`;
}

function decodeBody(body) {
  try {
    const binary = atob(body);
    return new TextDecoder().decode(Uint8Array.from(binary, (character) => character.charCodeAt(0)));
  } catch (_) { return String(body); }
}

function formatDuration(duration) {
  if (!duration) return 'Timing unavailable';
  const milliseconds = Number(duration) / 1000000;
  return milliseconds < 1 ? `${Math.round(Number(duration) / 1000)} μs` : `${milliseconds.toFixed(1)} ms`;
}

function escapeHTML(value) {
  return String(value).replace(/[&<>'"]/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  }[character]));
}

loadRequests();
connectEvents();

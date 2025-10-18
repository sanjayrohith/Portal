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
  state.requests.set(request.id, request);
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
    .then((request) => {
      const panel = document.querySelector('#detail-panel');
      panel.innerHTML = `<div class="detail-placeholder"><p>Request <strong>${escapeHTML(request.id)}</strong> selected.</p></div>`;
    })
    .catch(() => {});
}

function escapeHTML(value) {
  return String(value).replace(/[&<>'"]/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  }[character]));
}

loadRequests();
connectEvents();

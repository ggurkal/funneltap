const listEl = document.getElementById("request-list");
const detailEl = document.getElementById("detail");
const statusEl = document.getElementById("status");
const clearBtn = document.getElementById("clear-btn");

let selectedId = null;
let newestId = 0;
let knownIds = new Set();
let pollTimer = null;

const LOCALE = "nl-NL";

function formatTime(iso) {
  const d = new Date(iso);
  return d.toLocaleTimeString(LOCALE, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatDateTime(iso) {
  const d = new Date(iso);
  return d.toLocaleString(LOCALE, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function proxyListLabel(proxy) {
  if (!proxy.status) return "pending";
  return `${proxy.status} · ${proxy.durationMs}ms`;
}

function proxyStatusClass(proxy) {
  if (!proxy.status) return "status-pending";
  if (proxy.error) return "status-err";
  return "status-ok";
}

function createListItem(req) {
  const li = document.createElement("li");
  li.className = "request-item";
  li.dataset.id = String(req.id);
  if (req.id === selectedId) {
    li.classList.add("selected");
  }

  const line1 = document.createElement("div");
  line1.className = "line-1";

  const method = document.createElement("span");
  method.className = "method";
  method.textContent = req.method;

  const path = document.createElement("span");
  path.className = "path";
  path.textContent = req.path;

  line1.append(method, path);

  const line2 = document.createElement("div");
  line2.className = "line-2";

  const time = document.createElement("span");
  time.textContent = formatTime(req.receivedAt);

  const status = document.createElement("span");
  status.className = proxyStatusClass(req.proxy);
  status.textContent = proxyListLabel(req.proxy);

  line2.append(time, status);
  li.append(line1, line2);

  li.addEventListener("click", () => {
    selectedId = req.id;
    highlightSelected();
    loadDetail(req.id);
  });

  return li;
}

function prependRequests(requests) {
  for (const req of requests) {
    if (knownIds.has(req.id)) {
      updateListItem(req);
      continue;
    }
    knownIds.add(req.id);
    listEl.prepend(createListItem(req));
    if (req.id > newestId) {
      newestId = req.id;
    }
  }
}

function updateListItem(req) {
  const item = listEl.querySelector(`[data-id="${req.id}"]`);
  if (!item) return;

  const status = item.querySelector(".line-2 span:last-child");
  if (status) {
    status.className = proxyStatusClass(req.proxy);
    status.textContent = proxyListLabel(req.proxy);
  }
}

function highlightSelected() {
  for (const item of listEl.querySelectorAll(".request-item")) {
    item.classList.toggle("selected", Number(item.dataset.id) === selectedId);
  }
}

function escapeHtml(str) {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function decodeBody(bodyBase64) {
  if (!bodyBase64) return "";
  const binary = atob(bodyBase64);
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  return new TextDecoder("utf-8", { fatal: false }).decode(bytes);
}

function formatBody(body) {
  if (!body) return "";
  const trimmed = body.trim();
  if (!trimmed) return "";
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 4);
  } catch {
    return body;
  }
}

function detailStatusClass(status, error) {
  if (!status) return "detail-status-pending";
  if (error || status >= 500) return "detail-status-5xx";
  if (status >= 400) return "detail-status-4xx";
  if (status >= 200 && status < 300) return "detail-status-2xx";
  return "detail-status-other";
}

function renderBodyHtml(bodyBase64) {
  if (!bodyBase64) {
    return '<p class="body-empty">Empty body</p>';
  }
  const decoded = decodeBody(bodyBase64);
  if (!decoded) {
    return '<p class="body-empty">Empty body</p>';
  }
  return `<pre class="body-block">${escapeHtml(formatBody(decoded))}</pre>`;
}

function renderHeadersTable(headers) {
  const entries = Object.entries(headers || {});
  if (entries.length === 0) {
    return '<p class="muted">No headers</p>';
  }
  const rows = entries
    .map(([k, v]) => `<tr><th>${escapeHtml(k)}</th><td>${escapeHtml(v)}</td></tr>`)
    .join("");
  return `<table class="headers-table"><tbody>${rows}</tbody></table>`;
}

async function loadDetail(id) {
  try {
    const res = await fetch(`/requests/${id}`);
    if (!res.ok) throw new Error(res.statusText);
    const req = await res.json();
    renderDetail(req);
  } catch (err) {
    detailEl.innerHTML = `<div class="detail-empty">Failed to load request: ${escapeHtml(err.message)}</div>`;
  }
}

function renderDetail(req) {
  const proxy = req.proxy || {};
  const statusLabel = proxy.status ? String(proxy.status) : "pending";
  const statusClass = detailStatusClass(proxy.status, proxy.error);
  detailEl.innerHTML = `
    <div class="detail-header">
      <div class="detail-header-left">
        <span class="detail-status ${statusClass}">${escapeHtml(statusLabel)}</span>
        <span class="detail-method">${escapeHtml(req.method)}</span>
        <span class="detail-path">${escapeHtml(req.path)}</span>
      </div>
      <time class="detail-time">${escapeHtml(formatDateTime(req.receivedAt))}</time>
    </div>
    ${proxy.error ? `<p class="detail-proxy-error">${escapeHtml(proxy.error)}</p>` : ""}

    <section>
      <h2>Headers</h2>
      ${renderHeadersTable(req.headers)}
    </section>

    <section>
      <h2>Body</h2>
      ${renderBodyHtml(req.bodyBase64)}
    </section>
  `;
}

async function refreshPending() {
  const pending = [...listEl.querySelectorAll(".status-pending")]
    .map((el) => el.closest(".request-item")?.dataset.id)
    .filter(Boolean);
  if (pending.length === 0) return;

  const res = await fetch("/requests");
  if (!res.ok) return;
  const items = await res.json();
  for (const req of items) {
    if (pending.includes(String(req.id))) {
      updateListItem(req);
    }
  }
}

async function poll() {
  try {
    const url = newestId > 0 ? `/requests?after=${newestId}` : "/requests";
    const res = await fetch(url);
    if (!res.ok) throw new Error(res.statusText);
    const items = await res.json();

    if (newestId === 0) {
      listEl.innerHTML = "";
      knownIds.clear();
      for (const req of items) {
        knownIds.add(req.id);
        listEl.appendChild(createListItem(req));
        if (req.id > newestId) newestId = req.id;
      }
    } else {
      prependRequests(items);
    }

    await refreshPending();

    statusEl.textContent = `${knownIds.size} request(s) · polling every 1s`;
  } catch (err) {
    statusEl.textContent = `poll error: ${err.message}`;
  }
}

async function clearRequests() {
  try {
    const res = await fetch("/requests", { method: "DELETE" });
    if (!res.ok) throw new Error(res.statusText);
    listEl.innerHTML = "";
    knownIds.clear();
    newestId = 0;
    selectedId = null;
    detailEl.innerHTML = '<div class="detail-empty">Select a request from the list</div>';
    await poll();
  } catch (err) {
    statusEl.textContent = `clear error: ${err.message}`;
  }
}

clearBtn.addEventListener("click", clearRequests);

function syncThemeFromSystem() {
  const dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  const root = document.documentElement;
  root.classList.remove("dark", "light");
  root.classList.add(dark ? "dark" : "light");
}

window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", syncThemeFromSystem);

poll();
pollTimer = setInterval(poll, 1000);

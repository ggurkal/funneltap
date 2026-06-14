const listEl = document.getElementById("request-list");
const detailEl = document.getElementById("detail");
const statusEl = document.getElementById("status");
const clearBtn = document.getElementById("clear-btn");
const routesBtn = document.getElementById("routes-btn");
const routeFilterEl = document.getElementById("route-filter");
const routesDialog = document.getElementById("routes-dialog");
const routesListEl = document.getElementById("routes-list");
const routesErrorEl = document.getElementById("routes-error");
const addRouteForm = document.getElementById("add-route-form");
const recoveryDialog = document.getElementById("recovery-dialog");
const recoveryMessageEl = document.getElementById("recovery-message");
const recoveryListEl = document.getElementById("recovery-list");
const recoveryDismissBtn = document.getElementById("recovery-dismiss");
const recoveryRestoreBtn = document.getElementById("recovery-restore");

let selectedId = null;
let selectedRouteId = "";
let newestId = 0;
let knownIds = new Set();
let pollTimer = null;
let routes = [];
let filterRouteId = "";

const LOCALE = "nl-NL";

function fnv1a32(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

// Curated route colors — blues, purples, teals, amber; no green/red (status colors).
const ROUTE_PALETTE = [
  "#5b8def",
  "#8b6fd4",
  "#3aa6c8",
  "#c49a3c",
  "#6b7fd4",
  "#4a90a4",
  "#9b7ed9",
  "#7a8fa6",
];

function routePaletteIndex(routeId) {
  return fnv1a32(routeId) % ROUTE_PALETTE.length;
}

function routeAccentColor(routeId) {
  return ROUTE_PALETTE[routePaletteIndex(routeId)];
}

function routeBadgeBackground(routeId) {
  const color = routeAccentColor(routeId);
  return `linear-gradient(to right, ${color}33 75%, transparent)`;
}

function applyRouteBadgeStyle(el, routeId) {
  el.style.color = routeAccentColor(routeId);
  el.style.background = routeBadgeBackground(routeId);
}

function routeLabel(req) {
  if (req.routePath) return req.routePath;
  if (req.target) return req.target;
  return "route";
}

function requestPathDisplay(req) {
  const path = req.path || "";
  if (req.routePath !== "/") return path;
  const q = path.indexOf("?");
  const pathOnly = q === -1 ? path : path.slice(0, q);
  const query = q === -1 ? "" : path.slice(q);
  if (!pathOnly || pathOnly === "/") return query;
  return (pathOnly.startsWith("/") ? pathOnly.slice(1) : pathOnly) + query;
}

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

function passesFilter(req) {
  return !filterRouteId || req.routeId === filterRouteId;
}

function selectionMatchesFilter() {
  if (!selectedId) return false;
  return !filterRouteId || selectedRouteId === filterRouteId;
}

function showEmptyDetail() {
  detailEl.innerHTML = '<div class="detail-empty">Select a request from the list</div>';
}

function clearSelection() {
  selectedId = null;
  selectedRouteId = "";
  showEmptyDetail();
}

function createListItem(req) {
  const li = document.createElement("li");
  li.className = "request-item";
  li.dataset.id = String(req.id);
  if (req.id === selectedId) {
    li.classList.add("selected");
  }
  if (!passesFilter(req)) {
    li.hidden = true;
  }

  const line1 = document.createElement("div");
  line1.className = "line-1";

  const method = document.createElement("span");
  method.className = "method";
  method.textContent = req.method;

  const path = document.createElement("span");
  path.className = "path";
  path.textContent = requestPathDisplay(req);

  const pathCombo = document.createElement("span");
  pathCombo.className = "path-combo";

  if (req.routeId) {
    const badge = document.createElement("span");
    badge.className = "route-badge";
    badge.textContent = routeLabel(req);
    applyRouteBadgeStyle(badge, req.routeId);
    pathCombo.append(badge);
  }

  pathCombo.append(path);
  line1.append(method, pathCombo);

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
    selectedRouteId = req.routeId || "";
    highlightSelected();
    loadDetail(req.id);
  });

  return li;
}

function prependRequests(requests) {
  for (const req of requests) {
    if (!passesFilter(req) && filterRouteId) {
      if (knownIds.has(req.id)) {
        const item = listEl.querySelector(`[data-id="${req.id}"]`);
        if (item) item.hidden = true;
      }
      continue;
    }
    if (knownIds.has(req.id)) {
      updateListItem(req);
      continue;
    }
    knownIds.add(req.id);
    const li = createListItem(req);
    li.dataset.routeId = req.routeId || "";
    listEl.prepend(li);
    if (req.id > newestId) {
      newestId = req.id;
    }
  }
}

function updateListItem(req) {
  const item = listEl.querySelector(`[data-id="${req.id}"]`);
  if (!item) return;
  item.dataset.routeId = req.routeId || "";
  item.hidden = !passesFilter(req);

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
  const routeBadge = req.routeId
    ? `<span class="route-badge detail-route-badge" style="color:${routeAccentColor(req.routeId)};background:${routeBadgeBackground(req.routeId)}">${escapeHtml(routeLabel(req))}</span>`
    : "";
  const pathBlock = req.routeId
    ? `<span class="path-combo">${routeBadge}<span class="detail-path">${escapeHtml(requestPathDisplay(req))}</span></span>`
    : `<span class="detail-path">${escapeHtml(req.path)}</span>`;
  detailEl.innerHTML = `
    <div class="detail-header">
      <div class="detail-header-left">
        <span class="detail-status ${statusClass}">${escapeHtml(statusLabel)}</span>
        <span class="detail-method">${escapeHtml(req.method)}</span>
        ${pathBlock}
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

  const url = filterRouteId ? `/requests?route=${encodeURIComponent(filterRouteId)}` : "/requests";
  const res = await fetch(url);
  if (!res.ok) return;
  const items = await res.json();
  for (const req of items) {
    if (!pending.includes(String(req.id))) continue;
    updateListItem(req);
    if (selectedId === req.id && req.proxy?.status) {
      await loadDetail(req.id);
    }
  }
}

async function poll() {
  try {
    let url = newestId > 0 ? `/requests?after=${newestId}` : "/requests";
    if (filterRouteId) {
      const sep = url.includes("?") ? "&" : "?";
      url += `${sep}route=${encodeURIComponent(filterRouteId)}`;
    }
    const res = await fetch(url);
    if (!res.ok) throw new Error(res.statusText);
    const items = await res.json();

    if (newestId === 0) {
      listEl.innerHTML = "";
      knownIds.clear();
      for (const req of items) {
        if (!passesFilter(req)) continue;
        knownIds.add(req.id);
        const li = createListItem(req);
        li.dataset.routeId = req.routeId || "";
        listEl.appendChild(li);
        if (req.id > newestId) newestId = req.id;
      }
    } else {
      prependRequests(items);
    }

    await refreshPending();

    statusEl.textContent = `${knownIds.size} request(s) · ${routes.length} route(s) · polling every 1s`;
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
    clearSelection();
    await poll();
  } catch (err) {
    statusEl.textContent = `clear error: ${err.message}`;
  }
}

function showRoutesError(msg) {
  if (!msg) {
    routesErrorEl.hidden = true;
    routesErrorEl.textContent = "";
    return;
  }
  routesErrorEl.hidden = false;
  routesErrorEl.textContent = msg;
}

function renderRoutesList() {
  routesListEl.innerHTML = "";
  if (routes.length === 0) {
    routesListEl.innerHTML = '<li class="routes-empty">No routes yet</li>';
    return;
  }
  for (const rt of routes) {
    const li = document.createElement("li");
    li.className = "routes-item";
    li.innerHTML = `
      <div class="routes-item-main">
        <strong>${escapeHtml(rt.path)}</strong>
        <span class="muted">→ ${escapeHtml(rt.target)}</span>
        ${rt.publicURL ? `<a href="${escapeHtml(rt.publicURL)}" target="_blank" rel="noopener">${escapeHtml(rt.publicURL)}</a>` : ""}
      </div>
      <button type="button" class="secondary outline routes-delete" data-id="${escapeHtml(rt.id)}">Delete</button>
    `;
    routesListEl.append(li);
  }
}

function syncRouteFilter() {
  const current = routeFilterEl.value;
  routeFilterEl.innerHTML = '<option value="">All routes</option>';
  for (const rt of routes) {
    const opt = document.createElement("option");
    opt.value = rt.id;
    opt.textContent = `${rt.path} → ${rt.target}`;
    routeFilterEl.append(opt);
  }
  if ([...routeFilterEl.options].some((o) => o.value === current)) {
    routeFilterEl.value = current;
  } else {
    routeFilterEl.value = "";
    filterRouteId = "";
  }
}

async function loadRoutes() {
  const res = await fetch("/routes");
  if (!res.ok) throw new Error(res.statusText);
  routes = await res.json();
  renderRoutesList();
  syncRouteFilter();
  await reloadRequests();
}

function resetRequestList({ clearSelection: shouldClearSelection = true } = {}) {
  listEl.innerHTML = "";
  knownIds.clear();
  newestId = 0;
  if (shouldClearSelection) {
    clearSelection();
  }
}

async function reloadRequests() {
  const keepSelection = selectionMatchesFilter();
  resetRequestList({ clearSelection: !keepSelection });
  await poll();
  if (!keepSelection) return;

  const item = listEl.querySelector(`[data-id="${selectedId}"]`);
  if (item) {
    highlightSelected();
    return;
  }
  clearSelection();
}

async function addRoute(path, target) {
  const res = await fetch("/routes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, target }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  return res.json();
}

async function deleteRoute(id) {
  const res = await fetch(`/routes/${id}`, { method: "DELETE" });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
}

async function checkRecovery() {
  const res = await fetch("/recovery");
  if (!res.ok) return;
  const data = await res.json();
  if (!data.available) return;

  const preview = data.routes || [];
  recoveryMessageEl.textContent = `Recover ${preview.length} route(s) from a crashed session?`;
  recoveryListEl.innerHTML = preview
    .map((rt) => `<li><strong>${escapeHtml(rt.path)}</strong> → ${escapeHtml(rt.target)}</li>`)
    .join("");
  recoveryDialog.showModal();
}

clearBtn.addEventListener("click", clearRequests);

routesBtn.addEventListener("click", async () => {
  showRoutesError("");
  try {
    await loadRoutes();
    routesDialog.showModal();
  } catch (err) {
    statusEl.textContent = `routes error: ${err.message}`;
  }
});

routesDialog.querySelector(".close-routes").addEventListener("click", () => {
  routesDialog.close();
});

routesDialog.addEventListener("click", (e) => {
  if (e.target === routesDialog) {
    routesDialog.close();
  }
});

addRouteForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  showRoutesError("");
  const path = document.getElementById("route-path").value;
  const target = document.getElementById("route-target").value;
  try {
    await addRoute(path, target);
    addRouteForm.reset();
    await loadRoutes();
  } catch (err) {
    showRoutesError(err.message);
  }
});

routesListEl.addEventListener("click", async (e) => {
  const btn = e.target.closest(".routes-delete");
  if (!btn) return;
  showRoutesError("");
  try {
    await deleteRoute(btn.dataset.id);
    await loadRoutes();
  } catch (err) {
    showRoutesError(err.message);
  }
});

routeFilterEl.addEventListener("change", async () => {
  filterRouteId = routeFilterEl.value;
  await reloadRequests();
});

recoveryDismissBtn.addEventListener("click", async () => {
  await fetch("/recovery/dismiss", { method: "POST" });
  recoveryDialog.close();
});

recoveryRestoreBtn.addEventListener("click", async () => {
  const res = await fetch("/recovery/restore", { method: "POST" });
  if (!res.ok) {
    recoveryMessageEl.textContent = await res.text();
    return;
  }
  recoveryDialog.close();
  await loadRoutes();
});

function syncThemeFromSystem() {
  const dark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  const root = document.documentElement;
  root.classList.remove("dark", "light");
  root.classList.add(dark ? "dark" : "light");
}

window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", syncThemeFromSystem);

(async function init() {
  await checkRecovery();
  try {
    await loadRoutes();
  } catch {
    routes = [];
    resetRequestList();
  }
  pollTimer = setInterval(poll, 1000);
})();

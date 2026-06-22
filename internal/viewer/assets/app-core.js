"use strict";

const POLL_MS = 5000;
// Kanban age thresholds: cards older than these on their current status get
// a subtle stale tone (border only, no full-card alarm coloring).
const STALE_IN_PROGRESS_MS = 5 * 86400000;
const STALE_AUDIT_MS = 2 * 86400000;
let state = {
  projectId: null,
  page: "tickets",
  ticketView: "kanban",
  kanbanFilter: { priority: "", kind: "", status: "", parent: "", owner: "", claim: "", team: "", blocked: "", evidence: "" },
  kanbanSort: "ts",
  treeFilter: { parent: "", kind: "", priority: "", status: "" },
  worklogFilter: { q: "", agent: "" },
  worklogSort: "newest",
  insightsCard: null,
  pageSig: {},
  projectsSig: "",
  verifyStrict: false,
};
let pollTimer = null;
let drawerOpenedAt = 0;

function $(id) { return document.getElementById(id); }
const ICON_PATHS = {
  moon: '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>',
  panel: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  block: '<circle cx="12" cy="12" r="9"/><path d="M5.64 5.64l12.72 12.72"/>',
};
function icon(name, extraClass) {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  svg.setAttribute("class", "icon icon-" + name + (extraClass ? " " + extraClass : ""));
  svg.setAttribute("aria-hidden", "true");
  svg.innerHTML = ICON_PATHS[name] || "";
  return svg;
}
function el(tag, attrs, ...children) {
  const e = document.createElement(tag);
  for (const k in attrs || {}) {
    if (k === "class") e.className = attrs[k];
    else if (k === "html") e.innerHTML = attrs[k];
    else if (k === "text") e.textContent = attrs[k];
    else if (k.startsWith("on") && typeof attrs[k] === "function") e.addEventListener(k.slice(2), attrs[k]);
    else if (attrs[k] != null) e.setAttribute(k, attrs[k]);
  }
  for (const c of children) {
    if (c == null) continue;
    e.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return e;
}
function fmtTS(ts) {
  if (!ts) return "—";
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toISOString().substring(0, 16).replace("T", " ") + "Z";
}
function isClaimStale(claimUntil) {
  if (!claimUntil) return false;
  const d = new Date(claimUntil);
  if (isNaN(d.getTime())) return false;
  return d.getTime() < Date.now();
}
function describeClaimAge(claimUntil) {
  if (!claimUntil) return "";
  const d = new Date(claimUntil);
  if (isNaN(d.getTime())) return "";
  const diffMs = d.getTime() - Date.now();
  const absMin = Math.max(1, Math.round(Math.abs(diffMs) / 60000));
  if (diffMs < 0) {
    if (absMin >= 60) return "expired " + Math.round(absMin / 60) + "h ago";
    return "expired " + absMin + "m ago";
  }
  if (absMin >= 60) return "expires in " + Math.round(absMin / 60) + "h";
  return "expires in " + absMin + "m";
}
// formatLatencyBucket renders an hour value as either "Xh" (integer hours, <24h)
// or "X.Yd" (one-decimal days). Sub-hour precision is intentionally dropped.
function formatLatencyBucket(hours) {
  if (!isFinite(hours) || hours <= 0) return "0h";
  if (hours < 24) return Math.round(hours) + "h";
  const days = hours / 24;
  return (Math.round(days * 10) / 10).toFixed(1) + "d";
}
async function getJSON(path) {
  const r = await fetch(path);
  if (!r.ok) throw new Error(path + " → " + r.status);
  return r.json();
}
function setLoading(container) {
  container.innerHTML = "";
  container.appendChild(el("div", { class: "state-loading", text: "loading…" }));
}
function setError(container, err) {
  container.innerHTML = "";
  container.appendChild(el("div", { class: "state-error", text: "API error: " + err.message }));
}
function uniqueSorted(items) {
  return [...new Set(items.filter(Boolean).map(String))].sort();
}
function selectControl(values, current, label, onChange) {
  const sel = el("select", { onchange: (e) => onChange(e.target.value) });
  for (const item of values) {
    const value = typeof item === "string" ? item : item.value;
    const text = typeof item === "string" ? item : item.text;
    const opt = el("option", { value, text: value ? text : label });
    if (value === current) opt.selected = true;
    sel.appendChild(opt);
  }
  return sel;
}
function clearInvalidSelection(bucket, key, allowed) {
  if (bucket[key] && !allowed.includes(bucket[key])) {
    bucket[key] = "";
    syncURL();
  }
}
function textControl(value, placeholder, onChange) {
  return el("input", { type: "search", value, placeholder, oninput: (e) => onChange(e.target.value) });
}
function shouldSkipRender(page, data, background) {
  const sig = JSON.stringify(data);
  if (background && state.pageSig[page] === sig) return true;
  state.pageSig[page] = sig;
  return false;
}

async function loadProjects(opts) {
  const background = Boolean(opts && opts.background);
  try {
    const projects = await getJSON("/api/projects");
    const sig = JSON.stringify(projects);
    if (!background || sig !== state.projectsSig) {
      state.projectsSig = sig;
      renderProjectList(projects);
    }
    const ids = projects.map((p) => p.project_id);
    if (state.projectId && !ids.includes(state.projectId)) {
      state.projectId = null;
    }
    if (!state.projectId && projects.length > 0) {
      selectProject(projects[0].project_id);
    } else if (state.projectId) {
      if (!background) loadHeader();
      loadPage({ background });
    }
  } catch (e) {
    const list = $("project-list");
    list.innerHTML = "";
    list.appendChild(el("li", { class: "meta", text: "API error" }));
  }
}

function renderProjectList(projects) {
  const list = $("project-list");
  list.innerHTML = "";
  if (projects.length === 0) {
    list.appendChild(el("li", { class: "meta", text: "no registered projects" }));
    return;
  }
  for (const p of projects) {
    const li = el("li", { "data-id": p.project_id, onclick: () => selectProject(p.project_id) });
    li.appendChild(el("div", { text: p.display || p.slug || p.project_id }));
    const parts = [];
    if (p.missing) parts.push("missing");
    else {
      parts.push(p.open_tickets + " active");
      if (p.total_tickets != null) parts.push(p.total_tickets + " total");
      if (p.recent_activity_ts) parts.push("last " + fmtTS(p.recent_activity_ts));
    }
    li.appendChild(el("span", { class: "meta", text: parts.join(" · ") }));
    if (p.project_id === state.projectId) li.classList.add("active");
    list.appendChild(li);
  }
}

function selectProject(id) {
  state.projectId = id;
  syncURL();
  document.querySelectorAll("#project-list li").forEach((li) => {
    li.classList.toggle("active", li.dataset.id === id);
  });
  loadHeader();
  loadPage();
}

function selectPage(page) {
  state.page = normalizePage(page);
  syncURL();
  document.querySelectorAll("#page-nav li").forEach((li) => {
    li.classList.toggle("active", li.dataset.page === state.page);
  });
  loadPage();
}

function normalizePage(page) {
  if (page === "kanban" || page === "tree") {
    state.ticketView = page;
    return "tickets";
  }
  return page || "tickets";
}

function syncURL() {
  const params = new URLSearchParams();
  if (state.projectId) params.set("project", state.projectId);
  if (state.page && state.page !== "tickets") params.set("page", state.page);
  if (state.page === "tickets" && state.ticketView !== "kanban") params.set("view", state.ticketView);
  if (state.kanbanFilter.priority) params.set("priority", state.kanbanFilter.priority);
  if (state.kanbanFilter.kind) params.set("kind", state.kanbanFilter.kind);
  if (state.kanbanFilter.status) params.set("status", state.kanbanFilter.status);
  if (state.kanbanFilter.parent) params.set("parent", state.kanbanFilter.parent);
  if (state.kanbanFilter.owner) params.set("owner", state.kanbanFilter.owner);
  if (state.kanbanFilter.claim) params.set("claim", state.kanbanFilter.claim);
  if (state.kanbanFilter.team) params.set("team", state.kanbanFilter.team);
  if (state.kanbanFilter.blocked) params.set("blocked", state.kanbanFilter.blocked);
  if (state.kanbanFilter.evidence) params.set("evidence", state.kanbanFilter.evidence);
  if (state.kanbanSort !== "ts") params.set("sort", state.kanbanSort);
  if (state.treeFilter.parent) params.set("tree_parent", state.treeFilter.parent);
  if (state.treeFilter.kind) params.set("tree_kind", state.treeFilter.kind);
  if (state.treeFilter.priority) params.set("tree_priority", state.treeFilter.priority);
  if (state.treeFilter.status) params.set("tree_status", state.treeFilter.status);
  if (state.worklogFilter.q) params.set("worklog_q", state.worklogFilter.q);
  if (state.worklogFilter.agent) params.set("worklog_agent", state.worklogFilter.agent);
  if (state.worklogSort !== "newest") params.set("worklog_sort", state.worklogSort);
  const qs = params.toString();
  history.replaceState(null, "", qs ? "?" + qs : location.pathname);
}

async function loadHeader() {
  if (!state.projectId) return;
  try {
    const p = await getJSON("/api/projects/" + encodeURIComponent(state.projectId));
    $("project-name").textContent = p.name || p.slug || p.project_id;
    $("project-display").textContent = p.display || "";
  } catch (e) {
    $("project-name").textContent = "—";
    $("project-display").textContent = "";
  }
}

async function loadPage(opts) {
  const background = Boolean(opts && opts.background);
  const page = $("page");
  if (!state.projectId) {
    page.innerHTML = "";
    page.appendChild(el("div", { class: "state-empty", text: "Pick a project from the sidebar." }));
    return;
  }
  page.className = "page-" + state.page + (state.page === "tickets" ? " page-" + state.ticketView : "");
  if (!background) setLoading(page);
  try {
    switch (state.page) {
      case "dashboard": await renderDashboard(page, background); break;
      case "tickets":   await renderTickets(page, background); break;
      case "audit":     await renderAudit(page, background); break;
      case "worklog":   await renderWorklog(page, background); break;
      case "insights":  await renderInsights(page, background); break;
      default:          page.innerHTML = ""; page.appendChild(el("div", { class: "state-empty", text: "Unknown page." }));
    }
  } catch (e) {
    setError(page, e);
  }
}

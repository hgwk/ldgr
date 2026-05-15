"use strict";

const POLL_MS = 5000;
// Kanban age thresholds: cards older than these on their current status get
// a subtle stale tone (border only, no full-card alarm coloring).
const STALE_IN_PROGRESS_MS = 5 * 86400000;
const STALE_AUDIT_MS = 2 * 86400000;
let state = {
  projectId: null,
  page: "dashboard",
  kanbanFilter: { priority: "", kind: "", status: "", parent: "", agent: "", blocked: "", evidence: "" },
  kanbanSort: "ts",
  treeFilter: { parent: "", kind: "", priority: "", status: "" },
  worklogFilter: { q: "", agent: "" },
  worklogSort: "newest",
  pageSig: {},
  projectsSig: "",
  verifyStrict: false,
};
let pollTimer = null;

function $(id) { return document.getElementById(id); }
const ICON_PATHS = {
  moon: '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>',
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
      if (p.recent_worklog_ts) parts.push("last " + fmtTS(p.recent_worklog_ts));
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
  state.page = page;
  syncURL();
  document.querySelectorAll("#page-nav li").forEach((li) => {
    li.classList.toggle("active", li.dataset.page === page);
  });
  loadPage();
}

function syncURL() {
  const params = new URLSearchParams();
  if (state.projectId) params.set("project", state.projectId);
  if (state.page && state.page !== "dashboard") params.set("page", state.page);
	  if (state.kanbanFilter.priority) params.set("priority", state.kanbanFilter.priority);
	  if (state.kanbanFilter.kind) params.set("kind", state.kanbanFilter.kind);
	  if (state.kanbanFilter.status) params.set("status", state.kanbanFilter.status);
	  if (state.kanbanFilter.parent) params.set("parent", state.kanbanFilter.parent);
	  if (state.kanbanFilter.agent) params.set("agent", state.kanbanFilter.agent);
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
  page.className = "page-" + state.page;
  if (!background) setLoading(page);
  try {
    switch (state.page) {
      case "dashboard": await renderDashboard(page, background); break;
      case "kanban":    await renderKanban(page, background); break;
      case "audit":     await renderAudit(page, background); break;
      case "tree":      await renderTree(page, background); break;
      case "worklog":   await renderWorklog(page, background); break;
      case "insights":  await renderInsights(page, background); break;
      default:          page.innerHTML = ""; page.appendChild(el("div", { class: "state-empty", text: "Unknown page." }));
    }
  } catch (e) {
    setError(page, e);
  }
}

/* Dashboard */
async function renderDashboard(root, background) {
  const d = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/dashboard");
  if (shouldSkipRender("dashboard", d, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Dashboard" }));

  // Hero: full-width overall progress.
  const prog = d.progress || {};
  const hero = el("div", { class: "hero" });
  const heroHead = el("div", { class: "hero-head" });
  heroHead.appendChild(el("div", { class: "hero-label", text: "Overall progress" }));
  heroHead.appendChild(el("div", { class: "hero-value", text: (prog.percent || 0) + "%" }));
  hero.appendChild(heroHead);
  hero.appendChild(el("div", { class: "progress hero-progress" },
    el("span", { style: "width: " + (prog.percent || 0) + "%" })
  ));
  hero.appendChild(el("div", { class: "hero-delta", text:
    (prog.done || 0) + " done · " + (prog.active || 0) + " active · " + (prog.cancelled || 0) + " cancelled" }));
  root.appendChild(hero);

  // Secondary metrics band.
  const band = el("div", { class: "metric-band" });

  const a = d.audit || {};
  band.appendChild(el("div", { class: "metric" },
    el("div", { class: "label", text: "Audit pipeline" }),
    el("div", { class: "value", text: (a.audit_ready || 0) }),
    el("div", { class: "delta", text: (a.changes_requested || 0) + " changes · " + (a.weak_done || 0) + " weak done" }),
  ));

  const h = d.health || {};
  band.appendChild(el("div", { class: "metric" },
    el("div", { class: "label", text: "Delivery health" }),
    el("div", { class: "value", text: (h.closed_without_worklog || 0) + (h.orphan_worklog || 0) + (h.invalidated || 0) + (h.missing_evidence || 0) }),
    el("div", { class: "delta", text: "closed " + (h.closed_without_worklog || 0) + " · orphan " + (h.orphan_worklog || 0) + " · inv " + (h.invalidated || 0) + " · noev " + (h.missing_evidence || 0) }),
  ));

  // Lifecycle latency tiles: cycle time + audit latency.
  const lc = d.lifecycle || {};
  const cycleCount = lc.completed_cycle_count || 0;
  const cycleTile = el("div", { class: "metric" });
  cycleTile.appendChild(el("div", { class: "label", text: "Cycle time" }));
  if (cycleCount === 0) {
    cycleTile.appendChild(el("div", { class: "value", text: "—" }));
    cycleTile.appendChild(el("div", { class: "delta", text: "no completed cycles" }));
  } else {
    cycleTile.appendChild(el("div", { class: "value", text: formatLatencyBucket(lc.median_cycle_hours || 0) }));
    cycleTile.appendChild(el("div", { class: "delta", text: "p90 " + formatLatencyBucket(lc.p90_cycle_hours || 0) + " · " + cycleCount + " done" }));
  }
  band.appendChild(cycleTile);

  const pendingAudit = lc.pending_audit_count || 0;
  const auditLatencyTile = el("div", { class: "metric" });
  auditLatencyTile.appendChild(el("div", { class: "label", text: "Audit latency" }));
  if (pendingAudit === 0 && cycleCount === 0) {
    auditLatencyTile.appendChild(el("div", { class: "value", text: "—" }));
    auditLatencyTile.appendChild(el("div", { class: "delta", text: "no audit history" }));
  } else {
    auditLatencyTile.appendChild(el("div", { class: "value", text: formatLatencyBucket(lc.median_audit_latency_hours || 0) }));
    let sub = "p90 " + formatLatencyBucket(lc.p90_audit_latency_hours || 0);
    if (pendingAudit > 0) sub += " · " + pendingAudit + " pending";
    auditLatencyTile.appendChild(el("div", { class: "delta", text: sub }));
  }
  band.appendChild(auditLatencyTile);

  // Stale claims tile (expired + near-expiring agent claims on non-terminal tickets).
  const sc = d.stale_claims || { expired: 0, near_expiring: 0, samples: [] };
  const scTotal = (sc.expired || 0) + (sc.near_expiring || 0);
  const scTile = el("div", { class: "metric stale-claims" + (scTotal === 0 ? " calm" : "") });
  scTile.appendChild(el("div", { class: "label", text: "Stale claims" }));
  scTile.appendChild(el("div", { class: "value", text: String(scTotal) }));
  scTile.appendChild(el("div", { class: "delta", text:
    scTotal === 0
      ? "no stale claims"
      : (sc.expired || 0) + " expired · " + (sc.near_expiring || 0) + " expiring soon"
  }));
  const samples = sc.samples || [];
  if (samples.length > 0) {
    const list = el("ul", { class: "stale-claims-samples" });
    for (const s of samples) {
      const li = el("li");
      const idLink = el("a", {
        class: "mono",
        href: "#",
        text: s.ticket_id || "—",
        onclick: (e) => { e.preventDefault(); openDrawer(s.ticket_id); },
      });
      li.appendChild(idLink);
      li.appendChild(document.createTextNode(" " + describeClaimAge(s.claim_until)));
      list.appendChild(li);
    }
    scTile.appendChild(list);
  }
  band.appendChild(scTile);
  root.appendChild(band);

  // Priority band
  const prio = d.priority || {};
  const pBand = el("div", { class: "metric-band" });
  const pTotal = (prio.p0||0) + (prio.p1||0) + (prio.p2||0) + (prio.p3||0);
  pBand.appendChild(el("div", { class: "metric" },
    el("div", { class: "label", text: "Active priorities" }),
    el("div", { class: "value", text: String(pTotal) }),
    el("div", { class: "delta", text: "P0 " + (prio.p0||0) + " · P1 " + (prio.p1||0) + " · P2 " + (prio.p2||0) + " · P3 " + (prio.p3||0) }),
  ));
  // Kind distribution as a single tile (text only).
  const kinds = d.kind || [];
  const kindText = kinds.length === 0 ? "—" : kinds.map(k => k.kind + ": " + k.count).join(" · ");
  pBand.appendChild(el("div", { class: "metric" },
    el("div", { class: "label", text: "Kind distribution" }),
    el("div", { class: "value", text: String(kinds.reduce((s,k)=>s+k.count, 0)) }),
    el("div", { class: "delta", text: kindText }),
  ));
  root.appendChild(pBand);

  // Active agents widget (24h window).
  const aa = d.active_agents || { agents: [], unknown_count: 0, window_hours: 24 };
  const aaAgents = aa.agents || [];
  const aaBand = el("div", { class: "metric-band" });
  const aaTile = el("div", { class: "metric active-agents" + (aaAgents.length === 0 ? " calm" : "") });
  aaTile.appendChild(el("div", { class: "label", text: "Active agents · last " + (aa.window_hours || 24) + "h" }));
  aaTile.appendChild(el("div", { class: "value", text: String(aaAgents.length) }));
  if (aaAgents.length === 0) {
    aaTile.appendChild(el("div", { class: "delta", text: aa.unknown_count > 0 ? (aa.unknown_count + " unknown rows") : "no recent activity" }));
  } else {
    const totalRows = aaAgents.reduce((s, a) => s + (a.rows || 0), 0);
    let sub = totalRows + " rows";
    if (aa.unknown_count > 0) sub += " · " + aa.unknown_count + " unknown";
    aaTile.appendChild(el("div", { class: "delta", text: sub }));
    const list = el("ul", { class: "active-agents-list" });
    for (const a of aaAgents) {
      const li = el("li");
      li.appendChild(el("span", { class: "mono", text: a.agent }));
      const tail = " · " + (a.rows || 0) + (a.role ? " · " + a.role : "");
      li.appendChild(document.createTextNode(tail));
      list.appendChild(li);
    }
    aaTile.appendChild(list);
  }
  aaBand.appendChild(aaTile);

  // Verify status widget (isolated fetch — failures don't poison the dashboard).
  const verifyTile = el("div", { class: "metric verify-status", id: "verify-status-tile" });
  verifyTile.appendChild(buildVerifyTileBody(null, null));
  aaBand.appendChild(verifyTile);
  root.appendChild(aaBand);
  refreshVerifyTile();

  // Parents table.
  root.appendChild(el("div", { class: "section-heading" }, el("h3", { text: "Parent completion" })));
  const parents = d.parents || [];
  if (parents.length === 0) {
    root.appendChild(el("div", { class: "state-empty", text: "No parents yet." }));
  } else {
    const table = el("table", { class: "dense" });
    table.appendChild(el("thead", null, el("tr", null,
      el("th", { text: "Parent" }), el("th", { text: "Done" }), el("th", { text: "Active" }),
      el("th", { text: "Blocked" }), el("th", { text: "Cancelled" }), el("th", { text: "%" })
    )));
    const tb = el("tbody");
    for (const p of parents) {
      tb.appendChild(el("tr", null,
        el("td", { class: "mono", text: p.parent || "—" }),
        el("td", { text: String(p.done) }),
        el("td", { text: String(p.active) }),
        el("td", { text: String(p.blocked) }),
        el("td", { text: String(p.cancelled) }),
        el("td", { text: (p.percent || 0) + "%" }),
      ));
    }
    table.appendChild(tb);
    root.appendChild(table);
  }

  // Recent activity.
  root.appendChild(el("div", { class: "section-heading" }, el("h3", { text: "Recent activity" })));
  const recent = d.recent || [];
  if (recent.length === 0) {
    root.appendChild(el("div", { class: "state-empty", text: "No activity yet." }));
  } else {
    const table = el("table", { class: "dense" });
    table.appendChild(el("thead", null, el("tr", null,
      el("th", { text: "When" }), el("th", { text: "Kind" }), el("th", { text: "Ticket" }), el("th", { text: "Status / result" }), el("th", { text: "Task" })
    )));
    const tb = el("tbody");
    for (const r of recent) {
      tb.appendChild(el("tr", null,
        el("td", { text: fmtTS(r.ts) }),
        el("td", { text: r.kind }),
        el("td", { class: "mono", text: r.ticket || "—" }),
        el("td", null, r.status ? el("span", { class: "pill " + r.status, text: r.status }) : document.createTextNode(r.result || "—")),
        el("td", { text: r.task || "" }),
      ));
    }
    table.appendChild(tb);
    root.appendChild(table);
  }
}

/* Verify status widget — isolated fetch so verify errors stay local. */
function buildVerifyTileBody(payload, errMsg) {
  const frag = document.createDocumentFragment();
  const head = el("div", { class: "verify-status-head" });
  head.appendChild(el("div", { class: "label", text: "Verify status" }));
  const toggle = el("div", { class: "verify-toggle" });
  const btnDefault = el("button", {
    class: state.verifyStrict ? "" : "active",
    text: "default",
    onclick: () => { if (state.verifyStrict) { state.verifyStrict = false; refreshVerifyTile(); } },
  });
  const btnStrict = el("button", {
    class: state.verifyStrict ? "active" : "",
    text: "strict",
    onclick: () => { if (!state.verifyStrict) { state.verifyStrict = true; refreshVerifyTile(); } },
  });
  toggle.appendChild(btnDefault);
  toggle.appendChild(btnStrict);
  head.appendChild(toggle);
  frag.appendChild(head);

  if (errMsg) {
    frag.appendChild(el("div", { class: "value", text: "—" }));
    frag.appendChild(el("div", { class: "delta", text: "verify: " + errMsg }));
    return frag;
  }
  if (!payload) {
    frag.appendChild(el("div", { class: "value", text: "…" }));
    frag.appendChild(el("div", { class: "delta", text: "loading" }));
    return frag;
  }

  const fails = payload.fail_count || 0;
  const warns = payload.warn_count || 0;
  frag.appendChild(el("div", { class: "value", text: fails + " / " + warns }));

  const ran = payload.ran_at ? describeAgo(payload.ran_at) : "";
  const sub = (fails === 0 && warns === 0)
    ? "clean" + (ran ? " · ran " + ran : "")
    : fails + " fail · " + warns + " warn" + (ran ? " · ran " + ran : "");
  frag.appendChild(el("div", { class: "delta", text: sub }));

  const byCode = payload.by_code || {};
  const codes = Object.keys(byCode).map(k => ({ code: k, count: byCode[k] }));
  codes.sort((a, b) => b.count - a.count || (a.code < b.code ? -1 : 1));
  const top = codes.slice(0, 3);
  if (top.length > 0) {
    const list = el("ul", { class: "verify-codes" });
    for (const c of top) {
      const li = el("li");
      li.appendChild(el("span", { class: "mono", text: c.code }));
      li.appendChild(document.createTextNode(" · " + c.count));
      list.appendChild(li);
    }
    frag.appendChild(list);
  }
  return frag;
}

function describeAgo(ts) {
  const d = new Date(ts);
  if (isNaN(d.getTime())) return "";
  const sec = Math.max(0, Math.round((Date.now() - d.getTime()) / 1000));
  if (sec < 60) return sec + "s ago";
  const min = Math.round(sec / 60);
  if (min < 60) return min + "m ago";
  const hr = Math.round(min / 60);
  return hr + "h ago";
}

async function refreshVerifyTile() {
  const tile = $("verify-status-tile");
  if (!tile) return;
  // Show loading state immediately on toggle / first call.
  tile.innerHTML = "";
  tile.classList.remove("calm", "fails");
  tile.appendChild(buildVerifyTileBody(null, null));
  const pid = state.projectId;
  const url = "/api/projects/" + encodeURIComponent(pid) + "/verify" + (state.verifyStrict ? "?strict=1" : "");
  let payload = null;
  let errMsg = null;
  try {
    payload = await getJSON(url);
  } catch (e) {
    errMsg = "API error";
  }
  // Tile may have been replaced by a navigation in the meantime.
  const live = $("verify-status-tile");
  if (!live) return;
  live.innerHTML = "";
  live.classList.remove("calm", "fails");
  if (payload) {
    if ((payload.fail_count || 0) > 0) live.classList.add("fails");
    if ((payload.fail_count || 0) === 0 && (payload.warn_count || 0) === 0) live.classList.add("calm");
  }
  live.appendChild(buildVerifyTileBody(payload, errMsg));
}

/* Kanban */
async function renderKanban(root, background) {
  const k = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/kanban");
  if (shouldSkipRender("kanban", k, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Kanban" }));

  const allTickets = [];
  for (const col of (k.columns || [])) for (const t of (col.tickets || [])) allTickets.push(t);
  const parents = uniqueSorted(allTickets.map((t) => t.parent_ticket || t.parent));
  const agents = uniqueSorted(allTickets.map((t) => t.claimed_by || t.owner || t.agent));
  const canonical = Array.isArray(k.grid) && k.grid.length > 0;
  const kindOptions = canonical
    ? ["", "epic", "plan", "issue", "task", "audit", "ops"]
    : ["", "plan", "issue", "task", "audit", "ops"];
  const stateOptions = canonical
    ? ["", "ready", "doing", "review", "done", "backlog", "blocked", "rework", "dropped"]
    : ["", "open", "in_progress", "blocked", "audit_ready", "changes_requested", "done", "cancelled"];
  clearInvalidSelection(state.kanbanFilter, "kind", kindOptions);
  clearInvalidSelection(state.kanbanFilter, "status", stateOptions);

  // Filter bar
  const bar = el("div", { class: "kanban-bar" });
  bar.appendChild(selectControl(["", "P0", "P1", "P2", "P3"].map(v => ({ value: v, text: v ? "Priority " + v : "" })), state.kanbanFilter.priority, "All priorities", (v) => { state.kanbanFilter.priority = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl(kindOptions.map(v => ({ value: v, text: v ? (canonical ? "Type " : "Kind ") + v : "" })), state.kanbanFilter.kind, canonical ? "All types" : "All kinds", (v) => { state.kanbanFilter.kind = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl(stateOptions.map(v => ({ value: v, text: v ? (canonical ? "State " : "Status ") + v : "" })), state.kanbanFilter.status, canonical ? "All states" : "All statuses", (v) => { state.kanbanFilter.status = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl(["", ...parents], state.kanbanFilter.parent, "All parents", (v) => { state.kanbanFilter.parent = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl(["", ...agents], state.kanbanFilter.agent, "All agents", (v) => { state.kanbanFilter.agent = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl([{value:"",text:""}, {value:"yes",text:"Blocked only"}], state.kanbanFilter.blocked, "Any blocker", (v) => { state.kanbanFilter.blocked = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl([{value:"",text:""}, {value:"present",text:"Evidence present"}, {value:"missing",text:"Evidence missing"}], state.kanbanFilter.evidence, "Any evidence", (v) => { state.kanbanFilter.evidence = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl([
    { value: "ts", text: "Sort by updated" },
    { value: "oldest", text: "Sort by oldest" },
    { value: "priority", text: "Sort by priority" },
    { value: "parent", text: "Sort by parent" },
    { value: "blocked", text: "Sort blocked first" },
    { value: "missing_evidence", text: "Sort missing evidence" },
  ], state.kanbanSort, "Sort by updated", (v) => { state.kanbanSort = v; syncURL(); loadPage(); }));
  root.appendChild(bar);

  const cols = k.columns || [];
  if (cols.every((c) => (c.tickets || []).length === 0)) {
    root.appendChild(el("div", { class: "state-empty", text: "No tickets yet." }));
    return;
  }

  const byColumn = new Map(cols.map((c) => [c.id, c]));
  const orderedCols = canonical
    ? k.grid.flat().map((id) => byColumn.get(id)).filter(Boolean)
    : cols;
  const board = el("div", { class: "kanban-board" });
  for (const col of orderedCols) {
    const colEl = el("div", { class: "kanban-col" });
    const head = el("div", { class: "kanban-col-head" });
    head.appendChild(el("span", { class: "kanban-col-title", text: col.title }));

    let tickets = (col.tickets || []).filter((t) => {
	      if (state.kanbanFilter.priority && (t.priority || "") !== state.kanbanFilter.priority) return false;
	      if (state.kanbanFilter.kind && (t.kind || t.type || "") !== state.kanbanFilter.kind) return false;
	      if (state.kanbanFilter.status && ticketState(t) !== state.kanbanFilter.status) return false;
	      if (state.kanbanFilter.parent && (t.parent_ticket || t.parent || "") !== state.kanbanFilter.parent) return false;
	      if (state.kanbanFilter.agent && (t.claimed_by || t.owner || t.agent || "") !== state.kanbanFilter.agent) return false;
	      if (state.kanbanFilter.blocked === "yes" && !(t.blocked_by || []).some(Boolean)) return false;
	      if (state.kanbanFilter.evidence === "present" && !(t.evidence || []).some(Boolean)) return false;
	      if (state.kanbanFilter.evidence === "missing" && (t.evidence || []).some(Boolean)) return false;
	      return true;
	    });
	    const rank = { "P0": 0, "P1": 1, "P2": 2, "P3": 3 };
	    if (state.kanbanSort === "priority") {
	      tickets = tickets.slice().sort((a, b) => (rank[a.priority] ?? 9) - (rank[b.priority] ?? 9));
	    } else if (state.kanbanSort === "oldest") {
	      tickets = tickets.slice().sort((a, b) => (a.ts || "").localeCompare(b.ts || ""));
	    } else if (state.kanbanSort === "parent") {
	      tickets = tickets.slice().sort((a, b) => (a.parent_ticket || a.parent || "").localeCompare(b.parent_ticket || b.parent || ""));
	    } else if (state.kanbanSort === "blocked") {
	      tickets = tickets.slice().sort((a, b) => Number((b.blocked_by || []).some(Boolean)) - Number((a.blocked_by || []).some(Boolean)));
	    } else if (state.kanbanSort === "missing_evidence") {
	      tickets = tickets.slice().sort((a, b) => Number(!(b.evidence || []).some(Boolean)) - Number(!(a.evidence || []).some(Boolean)));
	    }

    head.appendChild(el("span", { class: "kanban-col-count", text: String(tickets.length) }));
    colEl.appendChild(head);

    const list = el("div", { class: "kanban-col-list" });
    for (const t of tickets) {
      list.appendChild(kanbanCard(t));
    }
    colEl.appendChild(list);
    board.appendChild(colEl);
  }
  root.appendChild(board);
}

// kanbanAgeTone returns CSS classes + an optional age chip label based on how
// long the ticket has been parked in its current status. Subtle by design.
function kanbanAgeTone(t) {
  if (!t || !t.ts) return { cls: "", chip: "" };
  const d = new Date(t.ts);
  if (isNaN(d.getTime())) return { cls: "", chip: "" };
  const ageMs = Date.now() - d.getTime();
  const stateName = ticketState(t);
  if ((stateName === "in_progress" || stateName === "doing") && ageMs >= STALE_IN_PROGRESS_MS) {
    return { cls: "kanban-card-stale", chip: Math.floor(ageMs / 86400000) + "d" };
  }
  if ((stateName === "audit_ready" || stateName === "review") && ageMs >= STALE_AUDIT_MS) {
    return { cls: "kanban-card-audit-stale", chip: Math.floor(ageMs / 86400000) + "d" };
  }
  return { cls: "", chip: "" };
}

function kanbanCard(t) {
  const tone = kanbanAgeTone(t);
  const id = ticketID(t);
  const stateName = ticketState(t);
  const card = el("div", { class: "kanban-card" + (tone.cls ? " " + tone.cls : ""), onclick: () => openDrawer(id) });

  const top = el("div", { class: "kanban-card-top" });
  const idGroup = el("span", { class: "kanban-card-idgroup" });
  idGroup.appendChild(el("span", { class: "mono kanban-card-id", text: id || "—" }));
  if (stateName) idGroup.appendChild(el("span", { class: "pill " + stateName, text: stateName }));
  if (tone.chip) {
    idGroup.appendChild(el("span", {
      class: "kanban-age-chip " + tone.cls + "-chip",
      title: stateName === "audit_ready" || stateName === "review" ? "audit-stale" : "in-progress stale",
      text: tone.chip,
    }));
  }
  top.appendChild(idGroup);
  const ownerName = t.claimed_by || t.owner || t.agent || "";
  if (ownerName) {
    const stale = isClaimStale(t.claim_until);
    const ownerBadge = el("span", { class: "badge kanban-owner" + (stale ? " kanban-owner-stale" : ""), title: stale ? "claim expired" : "owner" });
    ownerBadge.appendChild(el("span", { class: "kanban-owner-name", text: "@" + ownerName }));
    if (stale) {
      const mark = el("span", { class: "kanban-owner-mark", "aria-label": "claim expired" });
      mark.appendChild(icon("clock"));
      ownerBadge.appendChild(mark);
    }
    top.appendChild(ownerBadge);
  }
  card.appendChild(top);

  const task = el("div", { class: "kanban-card-task", text: ticketTitle(t) });
  card.appendChild(task);

  const badges = el("div", { class: "kanban-badges" });
  if (t.priority) badges.appendChild(el("span", { class: "badge badge-prio badge-prio-" + t.priority.toLowerCase(), text: t.priority }));
  const kind = t.kind || t.type || "";
  const area = t.category || t.area || "";
  if (kind && kind !== "task") badges.appendChild(el("span", { class: "badge", text: kind }));
  if (area) badges.appendChild(el("span", { class: "badge", text: area }));
  const blocked = (t.blocked_by || []).filter((s) => s);
  if (blocked.length > 0) {
    const b = el("span", { class: "badge badge-warn" });
    b.appendChild(icon("block"));
    b.appendChild(document.createTextNode(" " + blocked.length));
    badges.appendChild(b);
  }
  if ((t.evidence || []).length > 0) {
    const b = el("span", { class: "badge badge-ok" });
    b.appendChild(icon("check"));
    b.appendChild(document.createTextNode(" ev"));
    badges.appendChild(b);
  }
  const eventResult = t.event && t.event.result;
  if (t.audit_result === "pass" || eventResult === "pass") {
    const b = el("span", { class: "badge badge-ok" });
    b.appendChild(icon("check"));
    b.appendChild(document.createTextNode(" audit"));
    badges.appendChild(b);
  }
  if (t.branch) badges.appendChild(el("span", { class: "badge badge-mono", text: t.branch }));
  if (badges.childNodes.length > 0) card.appendChild(badges);

  return card;
}

function ticketID(t) { return (t && (t.ticket || t.id)) || ""; }
function ticketState(t) { return (t && (t.state || t.status)) || ""; }
function ticketTitle(t) { return (t && (t.task || t.title)) || ""; }

/* Audit queue */
function describeAge(ts) {
  if (!ts) return "—";
  const d = new Date(ts);
  if (isNaN(d.getTime())) return "—";
  const diffMs = Date.now() - d.getTime();
  if (diffMs < 0) return "waiting <1m";
  const min = Math.max(1, Math.round(diffMs / 60000));
  if (min < 60) return "waiting " + min + "m";
  const hr = Math.round(min / 60);
  if (hr < 48) return "waiting " + hr + "h";
  const day = Math.round(hr / 24);
  return "waiting " + day + "d";
}
async function renderAudit(root, background) {
  const q = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/audit-queue");
  if (shouldSkipRender("audit", q, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Audit queue" }));
  const items = (q && q.items) || [];
  root.appendChild(el("div", { class: "muted audit-subtitle", text: items.length + " " + (items.length === 1 ? "ticket" : "tickets") + " waiting for audit" }));

  if (items.length === 0) {
    root.appendChild(el("div", { class: "state-empty audit-empty", text: "No tickets are waiting for audit. Nice." }));
    return;
  }

  const table = el("table", { class: "dense audit-table" });
  table.appendChild(el("thead", null, el("tr", null,
    el("th", { text: "Ticket" }),
    el("th", { text: "Task" }),
    el("th", { text: "Priority" }),
    el("th", { text: "Age" }),
    el("th", { text: "Owner" }),
    el("th", { text: "Evidence" }),
  )));
  const tb = el("tbody");
  for (const it of items) {
    const tr = el("tr", { class: "audit-row", onclick: () => openDrawer(it.ticket_id) });
    tr.appendChild(el("td", { class: "mono", text: it.ticket_id || "—" }));
    const task = it.task || "";
    const truncated = task.length > 80 ? task.slice(0, 77) + "…" : task;
    tr.appendChild(el("td", { class: "audit-task", title: task, text: truncated }));
    const prio = it.priority || "P2";
    tr.appendChild(el("td", null,
      el("span", { class: "badge badge-prio badge-prio-" + prio.toLowerCase(), text: prio })
    ));
    tr.appendChild(el("td", { text: describeAge(it.waiting_since) }));
    const owner = it.claimed_by || it.agent || "";
    const ownerCell = el("td");
    if (owner) {
      ownerCell.appendChild(el("span", { class: "badge kanban-owner", text: "@" + owner }));
    } else {
      ownerCell.appendChild(document.createTextNode("—"));
    }
    tr.appendChild(ownerCell);
    const evCell = el("td");
    if (it.has_evidence) {
      const b = el("span", { class: "badge badge-ok" });
      b.appendChild(icon("check"));
      b.appendChild(document.createTextNode(" evidence"));
      evCell.appendChild(b);
    } else {
      evCell.appendChild(el("span", { class: "badge badge-warn", text: "no evidence" }));
    }
    tr.appendChild(evCell);
    tb.appendChild(tr);
  }
  table.appendChild(tb);
  root.appendChild(table);
}

async function openDrawer(ticketId) {
  const drawer = $("drawer");
  const body = $("drawer-body");
  drawer.classList.add("open");
  drawer.setAttribute("aria-hidden", "false");
  body.innerHTML = "";
  body.appendChild(el("div", { class: "state-loading", text: "loading…" }));
  try {
    const data = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/tickets/" + encodeURIComponent(ticketId));
    body.innerHTML = "";
    body.appendChild(renderDrawerHeader(data));
    body.appendChild(renderDrawerSummary(data.latest || {}, data.invalidated_via, data.history || []));
    body.appendChild(renderDrawerDetails(data.latest || {}, data.history || []));
    body.appendChild(renderDrawerHistory(data.history || []));
    body.appendChild(renderDrawerWorklogs(data.worklog || []));
    body.appendChild(renderDrawerGuidance(ticketId));
  } catch (e) {
    body.innerHTML = "";
    body.appendChild(el("div", { class: "state-error", text: "Could not load ticket: " + e.message }));
  }
}

function renderDrawerHeader(d) {
  const head = el("div", { class: "drawer-head" });
  const latest = d.latest || {};
  head.appendChild(el("span", { class: "mono drawer-id", text: d.ticket || latest.id || "—" }));
  const stateName = ticketState(latest);
  if (stateName) head.appendChild(el("span", { class: "pill " + stateName, text: stateName }));
  return head;
}

function renderDrawerSummary(latest, invalidatedVia, history) {
  const wrap = el("div", { class: "drawer-summary" });
  if (ticketTitle(latest)) wrap.appendChild(el("div", { class: "drawer-task", text: ticketTitle(latest) }));
  const event = latest.event || {};
  const rows = [
    ["parent",      latest.parent_ticket || latest.parent],
    ["category",    latest.category || latest.area],
    ["type",        latest.kind || latest.type],
    ["branch",      latest.branch],
    ["claimed_by",  latest.claimed_by],
    ["claim_until", latest.claim_until ? fmtTS(latest.claim_until) : ""],
    ["handoff_to",  latest.handoff_to],
    ["agent",       latest.agent || latest.owner],
    ["role",        latest.role || event.role],
    ["event.result", event.result],
  ];
  const dl = el("dl", { class: "drawer-meta" });
  for (const [k, v] of rows) {
    if (!v) continue;
    dl.appendChild(el("dt", { text: k }));
    dl.appendChild(el("dd", { class: k === "branch" ? "mono" : "", text: String(v) }));
  }
  const blocked = (latest.blocked_by || []).filter((s) => s);
  if (blocked.length > 0) {
    dl.appendChild(el("dt", { text: "blocked_by" }));
    const dd = el("dd");
    for (const b of blocked) dd.appendChild(el("span", { class: "badge badge-warn", text: b }));
    dl.appendChild(dd);
  }
  const reviewedN = latest.reviewed_n != null ? latest.reviewed_n : event.reviewed_n;
  if (reviewedN != null) {
    dl.appendChild(el("dt", { text: "reviewed_n" }));
    const dd = el("dd");
    const targetN = Number(reviewedN);
    const inHistory = Array.isArray(history) && history.some((r) => Number(r && r.n) === targetN);
    if (inHistory) {
      const a = el("a", {
        href: "#history-n-" + targetN,
        class: "mono drawer-backlink",
        text: "n=" + targetN,
        onclick: (ev) => { ev.preventDefault(); highlightHistoryRow(targetN); },
      });
      dd.appendChild(a);
    } else {
      dd.appendChild(el("span", { class: "mono", text: "n=" + targetN }));
    }
    dl.appendChild(dd);
  }
  if (dl.childNodes.length > 0) wrap.appendChild(dl);
  if (invalidatedVia) {
    wrap.appendChild(el("div", { class: "state-error", text: "This row is invalidated by n=" + invalidatedVia }));
  }
  return wrap;
}

/* Parse provenance one-liner out of a notes string.
   Recognized keys: archived, borrow, reference, new, not_borrowed.
   Returns { markers: [{key, value}], rest: "leftover notes text" }. */
function parseProvenance(notes) {
  const out = { markers: [], rest: "" };
  if (!notes || typeof notes !== "string") return out;
  const keys = ["archived", "borrow", "reference", "new", "not_borrowed"];
  // Split on ';', then re-glue any segment that doesn't start with a known
  // `key=` anchor back onto the previous segment's value. This preserves
  // semicolons that legitimately appear inside a value (e.g.
  // `not_borrowed=different domain; old impl was npm-based`).
  const rawSegments = notes.split(";");
  const isAnchor = (s) => {
    const eq = s.indexOf("=");
    if (eq <= 0) return false;
    const k = s.slice(0, eq).trim();
    return keys.includes(k);
  };
  const glued = [];
  for (const raw of rawSegments) {
    const trimmed = raw.trim();
    if (!trimmed) continue;
    if (isAnchor(trimmed) || glued.length === 0) {
      glued.push(trimmed);
    } else {
      // Re-attach to previous segment with the original ';' separator.
      glued[glued.length - 1] = glued[glued.length - 1] + "; " + trimmed;
    }
  }
  const leftover = [];
  for (const seg of glued) {
    const eq = seg.indexOf("=");
    if (eq > 0) {
      const k = seg.slice(0, eq).trim();
      const v = seg.slice(eq + 1).trim();
      if (keys.includes(k)) {
        out.markers.push({ key: k, value: v });
        continue;
      }
    }
    leftover.push(seg);
  }
  out.rest = leftover.join("; ");
  return out;
}

function renderDrawerDetails(latest, history) {
  const wrap = el("div", { class: "drawer-details" });
  const event = latest.event || {};

  // notes (with provenance parsing)
  const notes = latest.notes || event.notes || "";
  if (notes) {
    const prov = parseProvenance(notes);
    const section = el("section", { class: "drawer-section" });
    section.appendChild(el("h4", { class: "drawer-section-title", text: "notes" }));
    if (prov.markers.length > 0) {
      const row = el("div", { class: "provenance-row" });
      for (const m of prov.markers) {
        const cls = m.key === "not_borrowed" ? "badge badge-warn" : "badge";
        row.appendChild(el("span", { class: cls + " provenance-marker", text: m.key + "=" + m.value }));
      }
      section.appendChild(row);
    }
    if (prov.rest) {
      section.appendChild(el("div", { class: "drawer-prose", tabindex: "0", text: prov.rest }));
    } else if (prov.markers.length === 0) {
      section.appendChild(el("div", { class: "drawer-prose", tabindex: "0", text: notes }));
    }
    wrap.appendChild(section);
  }

  // decision
  if (latest.decision) {
    const section = el("section", { class: "drawer-section" });
    section.appendChild(el("h4", { class: "drawer-section-title", text: "decision" }));
    section.appendChild(el("div", { class: "drawer-prose", tabindex: "0", text: latest.decision }));
    wrap.appendChild(section);
  }

  if (event.summary) {
    const section = el("section", { class: "drawer-section" });
    section.appendChild(el("h4", { class: "drawer-section-title", text: "event.summary" }));
    section.appendChild(el("div", { class: "drawer-prose", tabindex: "0", text: event.summary }));
    wrap.appendChild(section);
  }

  // audit_notes
  if (latest.audit_notes) {
    const section = el("section", { class: "drawer-section" });
    section.appendChild(el("h4", { class: "drawer-section-title", text: "audit_notes" }));
    section.appendChild(el("div", { class: "drawer-prose", tabindex: "0", text: latest.audit_notes }));
    wrap.appendChild(section);
  }

  // acceptance (array of strings)
  const acc = Array.isArray(latest.acceptance) ? latest.acceptance.filter((s) => s) : [];
  if (acc.length > 0) {
    const section = el("section", { class: "drawer-section" });
    section.appendChild(el("h4", { class: "drawer-section-title", text: "acceptance" }));
    const ul = el("ul", { class: "drawer-list" });
    for (const item of acc) ul.appendChild(el("li", { text: String(item) }));
    section.appendChild(ul);
    wrap.appendChild(section);
  }

  // handoff (free-form blob)
  if (latest.handoff) {
    const section = el("section", { class: "drawer-section" });
    section.appendChild(el("h4", { class: "drawer-section-title", text: "handoff" }));
    let text;
    if (typeof latest.handoff === "string") {
      text = latest.handoff;
    } else {
      try { text = JSON.stringify(latest.handoff, null, 2); }
      catch (e) { text = String(latest.handoff); }
    }
    section.appendChild(el("pre", { class: "drawer-pre", tabindex: "0", text: text }));
    wrap.appendChild(section);
  }

  if (wrap.childNodes.length === 0) return document.createDocumentFragment();
  return wrap;
}

function highlightHistoryRow(n) {
  const tr = document.getElementById("history-n-" + n);
  if (!tr) return;
  tr.scrollIntoView({ block: "nearest", behavior: "smooth" });
  tr.classList.remove("history-flash");
  // force reflow so the animation restarts.
  void tr.offsetWidth;
  tr.classList.add("history-flash");
}

function renderDrawerHistory(history) {
  const wrap = el("div");
  wrap.appendChild(el("div", { class: "section-heading" }, el("h3", { text: "History" })));
  if (history.length === 0) {
    wrap.appendChild(el("div", { class: "state-empty", text: "No history." }));
    return wrap;
  }
  const table = el("table", { class: "dense" });
  table.appendChild(el("thead", null, el("tr", null,
    el("th", { text: "n" }), el("th", { text: "When" }), el("th", { text: "Status" }), el("th", { text: "Role" }), el("th", { text: "Note" })
  )));
  const tb = el("tbody");
  // Newest first.
  const rows = [...history].reverse();
  for (const r of rows) {
    const trAttrs = (r && r.n != null) ? { id: "history-n-" + r.n } : null;
    tb.appendChild(el("tr", trAttrs,
      el("td", { class: "mono", text: r.n != null ? String(r.n) : "" }),
      el("td", { text: fmtTS(r.ts) }),
      el("td", null, r.status ? el("span", { class: "pill " + r.status, text: r.status }) : document.createTextNode("")),
      el("td", { text: r.role || "" }),
      el("td", { text: r.notes || r.decision || r.audit_notes || "" }),
    ));
  }
  table.appendChild(tb);
  wrap.appendChild(table);
  return wrap;
}

function renderDrawerWorklogs(worklog) {
  const wrap = el("div");
  wrap.appendChild(el("div", { class: "section-heading" }, el("h3", { text: "Worklog" })));
  if (worklog.length === 0) {
    wrap.appendChild(el("div", { class: "state-empty", text: "No worklog rows for this ticket." }));
    return wrap;
  }
  const table = el("table", { class: "dense" });
  table.appendChild(el("thead", null, el("tr", null,
    el("th", { text: "When" }), el("th", { text: "Task" }), el("th", { text: "Result" }), el("th", { text: "Agent" })
  )));
  const tb = el("tbody");
  for (const r of worklog) {
    tb.appendChild(el("tr", null,
      el("td", { text: fmtTS(r.ts) }),
      el("td", { text: r.task || "" }),
      el("td", { text: r.result || "" }),
      el("td", { text: r.agent || "" }),
    ));
  }
  table.appendChild(tb);
  wrap.appendChild(table);
  return wrap;
}

function renderDrawerGuidance(ticketId) {
  const wrap = el("div", { class: "drawer-guidance" });
  wrap.appendChild(el("div", { class: "section-heading" }, el("h3", { text: "Next" })));
  const lines = [
    "ldgr next --ticket " + ticketId,
    "ldgr suggest commit --ticket " + ticketId,
    "ldgr suggest worklog --ticket " + ticketId,
  ];
  const pre = el("pre", { class: "guidance-pre" });
  pre.textContent = lines.join("\n");
  wrap.appendChild(pre);
  return wrap;
}

/* Tree */
async function renderTree(root, background) {
  const t = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/tickets");
  if (shouldSkipRender("tree", t, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Tree" }));

  // Flatten the buckets into one list of latest rows.
  const all = [];
  for (const bucket of (t.tree || [])) {
    for (const row of (bucket.tickets || [])) {
      all.push({ row, parent: bucket.parent });
    }
  }
	  if (all.length === 0) {
    root.appendChild(el("div", { class: "state-empty", text: "No tickets yet." }));
    return;
	  }
  const treeParents = uniqueSorted(all.map((it) => it.row.parent_ticket || it.parent));
  const treeBar = el("div", { class: "kanban-bar" });
  treeBar.appendChild(selectControl(["", ...treeParents], state.treeFilter.parent, "All parents", (v) => { state.treeFilter.parent = v; syncURL(); loadPage(); }));
  treeBar.appendChild(selectControl(["", "plan", "issue", "task", "audit", "ops"].map(v => ({ value: v, text: v ? "Kind " + v : "" })), state.treeFilter.kind, "All kinds", (v) => { state.treeFilter.kind = v; syncURL(); loadPage(); }));
  treeBar.appendChild(selectControl(["", "P0", "P1", "P2", "P3"].map(v => ({ value: v, text: v ? "Priority " + v : "" })), state.treeFilter.priority, "All priorities", (v) => { state.treeFilter.priority = v; syncURL(); loadPage(); }));
  treeBar.appendChild(selectControl(["", "open", "in_progress", "blocked", "audit_ready", "changes_requested", "done", "cancelled"].map(v => ({ value: v, text: v ? "Status " + v : "" })), state.treeFilter.status, "All statuses", (v) => { state.treeFilter.status = v; syncURL(); loadPage(); }));
  root.appendChild(treeBar);

  const byId = new Map();
  for (const item of all) byId.set(item.row.ticket, item);
  const childrenOf = new Map();
  const workstreamBuckets = new Map();
  for (const item of all) {
    const p = item.row.parent_ticket || item.parent || "—";
    if (byId.has(p)) {
      // parent is itself a ticket id → nested.
      if (!childrenOf.has(p)) childrenOf.set(p, []);
      childrenOf.get(p).push(item);
    } else {
      // parent is a workstream label.
      if (!workstreamBuckets.has(p)) workstreamBuckets.set(p, []);
      workstreamBuckets.get(p).push(item);
    }
  }
  const visible = computeVisibleTreeTickets(all, byId);

  // A ticket that has a ticket-parent shouldn't ALSO appear at the top of its
  // workstream bucket — exclude such tickets from workstream listings.
  for (const [bucket, items] of workstreamBuckets) {
    workstreamBuckets.set(bucket, items.filter((it) => visible.has(it.row.ticket) && !byId.has(it.row.parent_ticket)));
  }

  // Render each workstream bucket as a section, with each top-level ticket
  // optionally expanding into its nested children.
  const sortedBuckets = [...workstreamBuckets.keys()].sort();
  for (const parent of sortedBuckets) {
    const items = workstreamBuckets.get(parent);
    if (items.length === 0) continue;
    root.appendChild(el("div", { class: "section-heading" }, el("h3", { text: parent })));
    const list = el("div", { class: "tree-list" });
    for (const item of items) {
      list.appendChild(renderTreeNode(item.row, childrenOf, byId, visible, 0));
    }
    root.appendChild(list);
  }
}

function computeVisibleTreeTickets(items, byId) {
  const visible = new Set();
  const hasFilter = state.treeFilter.parent || state.treeFilter.kind || state.treeFilter.priority || state.treeFilter.status;
  for (const item of items) {
    if (!hasFilter || treeItemMatches(item)) markVisibleWithAncestors(item, byId, visible);
  }
  return visible;
}

function treeItemMatches(item) {
  if (state.treeFilter.parent && (item.row.parent_ticket || item.parent || "") !== state.treeFilter.parent) return false;
  if (state.treeFilter.kind && (item.row.kind || "") !== state.treeFilter.kind) return false;
  if (state.treeFilter.priority && (item.row.priority || "") !== state.treeFilter.priority) return false;
  if (state.treeFilter.status && (item.row.status || "") !== state.treeFilter.status) return false;
  return true;
}

function markVisibleWithAncestors(item, byId, visible) {
  let cur = item;
  while (cur && cur.row && cur.row.ticket && !visible.has(cur.row.ticket)) {
    visible.add(cur.row.ticket);
    cur = byId.get(cur.row.parent_ticket);
  }
}

function renderTreeNode(row, childrenOf, byId, visible, depth) {
  if (!visible.has(row.ticket)) return document.createDocumentFragment();
  const wrap = el("div", { class: "tree-node", style: "margin-left: " + (depth * 16) + "px" });
  const head = el("div", { class: "tree-node-head", onclick: () => openDrawer(row.ticket) });
  head.appendChild(el("span", { class: "mono", text: row.ticket }));
  if (row.priority) head.appendChild(el("span", { class: "badge badge-prio badge-prio-" + (row.priority||"").toLowerCase(), text: row.priority }));
  if (row.kind && row.kind !== "task") head.appendChild(el("span", { class: "badge", text: row.kind }));
  head.appendChild(el("span", { class: "pill " + (row.status || ""), text: row.status || "" }));
  head.appendChild(el("span", { class: "tree-task", text: row.task || "" }));
  head.appendChild(el("span", { class: "tree-ts muted", text: fmtTS(row.ts) }));
  wrap.appendChild(head);

  const kids = childrenOf.get(row.ticket) || [];
  // Sort children by ts desc.
  kids.sort((a, b) => (b.row.ts || "").localeCompare(a.row.ts || ""));
  for (const child of kids) {
    wrap.appendChild(renderTreeNode(child.row, childrenOf, byId, visible, depth + 1));
  }
  return wrap;
}

/* Worklog */
async function renderWorklog(root, background) {
  const w = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/worklog");
  if (shouldSkipRender("worklog", w, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Worklog" }));
  const allRows = w.rows || [];
  const agents = uniqueSorted(allRows.map((r) => r.agent));
  const bar = el("div", { class: "kanban-bar" });
  bar.appendChild(textControl(state.worklogFilter.q, "Search ticket/task/result", (v) => { state.worklogFilter.q = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl(["", ...agents], state.worklogFilter.agent, "All agents", (v) => { state.worklogFilter.agent = v; syncURL(); loadPage(); }));
  bar.appendChild(selectControl([{ value: "newest", text: "Newest first" }, { value: "oldest", text: "Oldest first" }], state.worklogSort, "Newest first", (v) => { state.worklogSort = v; syncURL(); loadPage(); }));
  root.appendChild(bar);
  let rows = allRows.filter((r) => {
    if (state.worklogFilter.agent && (r.agent || "") !== state.worklogFilter.agent) return false;
    const q = state.worklogFilter.q.trim().toLowerCase();
    if (q) {
      const hay = [r.ticket, r.task, r.result].join(" ").toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  });
  if (state.worklogSort === "oldest") rows = rows.slice().sort((a, b) => (a.ts || "").localeCompare(b.ts || ""));
  if (rows.length === 0) {
    root.appendChild(el("div", { class: "state-empty", text: "No worklog entries." }));
    return;
  }
  const table = el("table", { class: "dense" });
  table.appendChild(el("thead", null, el("tr", null,
    el("th", { text: "When" }), el("th", { text: "Ticket" }), el("th", { text: "Task" }), el("th", { text: "Result" }), el("th", { text: "Agent" })
  )));
  const tb = el("tbody");
  for (const r of rows.slice(0, 100)) {
    tb.appendChild(el("tr", null,
      el("td", { text: fmtTS(r.ts) }),
      el("td", { class: "mono", text: r.ticket || "—" }),
      el("td", { text: r.task || "" }),
      el("td", { text: r.result || "" }),
      el("td", { text: r.agent || "" }),
    ));
  }
  table.appendChild(tb);
  root.appendChild(table);
}

/* Insights */
async function renderInsights(root, background) {
  const ins = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/insights");
  if (shouldSkipRender("insights", ins, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Insights" }));
  const cards = [
    { key: "readyQueue", title: "Ready to start", render: (it) => it.ticket + " — " + (it.task || "") },
    { key: "topBlockers", title: "Top blockers", render: (it) => it.ticket + " (" + (it.dependents || []).length + " dep · " + it.status + ")" },
    { key: "staleInProgress", title: "Stale in_progress", render: (it) => it.ticket + " · " + Math.round(it.age_ms / 36e5) + "h" },
    { key: "closedWithoutWorklog", title: "Closed without worklog", render: (it) => it.ticket },
    { key: "worklogsWithoutTicket", title: "Orphan worklog", render: (it) => "n=" + it.n + " ticket=" + (it.ticket || "?") },
    { key: "invalidated", title: "Invalidated rows", render: (it) => "n=" + it.n + " ← " + it.via_n + " (" + it.kind + ")" },
  ];
  let any = false;
  for (const c of cards) {
    const items = ins[c.key] || [];
    if (items.length === 0) continue;
    any = true;
    root.appendChild(el("div", { class: "section-heading" }, el("h3", { text: c.title })));
    const ul = el("table", { class: "dense" });
    const tb = el("tbody");
    for (const it of items) tb.appendChild(el("tr", null, el("td", { text: c.render(it) })));
    ul.appendChild(tb);
    root.appendChild(ul);
  }
  if (!any) root.appendChild(el("div", { class: "state-empty", text: "All clean — no insights at the moment." }));
}

/* Init + polling */
function applyTheme(theme) {
  const dark = theme === "dark";
  document.documentElement.classList.toggle("dark", dark);
  const btn = document.getElementById("theme-toggle");
  if (btn) {
    btn.textContent = "";
    btn.appendChild(icon(dark ? "sun" : "moon"));
    btn.setAttribute("aria-pressed", dark ? "true" : "false");
  }
}
function bind() {
  document.querySelectorAll("#page-nav li").forEach((li) => {
    li.addEventListener("click", () => selectPage(li.dataset.page));
  });
  const closeBtn = document.querySelector("#drawer .close");
  if (closeBtn) {
    closeBtn.textContent = "";
    closeBtn.appendChild(icon("x"));
    closeBtn.addEventListener("click", () => $("drawer").classList.remove("open"));
  }
  const themeBtn = document.getElementById("theme-toggle");
  if (themeBtn) {
    themeBtn.addEventListener("click", () => {
      const next = document.documentElement.classList.contains("dark") ? "light" : "dark";
      try { localStorage.setItem("ldgr.theme", next); } catch (_) {}
      applyTheme(next);
    });
  }
}
function startPolling() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(() => loadProjects({ background: true }), POLL_MS);
}
(function init() {
  let savedTheme = null;
  try { savedTheme = localStorage.getItem("ldgr.theme"); } catch (_) {}
  if (!savedTheme && window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) {
    savedTheme = "dark";
  }
  applyTheme(savedTheme === "dark" ? "dark" : "light");
  const params = new URLSearchParams(location.search);
  if (params.get("project")) state.projectId = params.get("project");
  if (params.get("page")) state.page = params.get("page");
	  if (params.get("priority")) state.kanbanFilter.priority = params.get("priority");
	  if (params.get("kind")) state.kanbanFilter.kind = params.get("kind");
	  if (params.get("status")) state.kanbanFilter.status = params.get("status");
	  if (params.get("parent")) state.kanbanFilter.parent = params.get("parent");
	  if (params.get("agent")) state.kanbanFilter.agent = params.get("agent");
	  if (params.get("blocked")) state.kanbanFilter.blocked = params.get("blocked");
	  if (params.get("evidence")) state.kanbanFilter.evidence = params.get("evidence");
	  if (params.get("sort")) state.kanbanSort = params.get("sort");
	  if (params.get("tree_parent")) state.treeFilter.parent = params.get("tree_parent");
	  if (params.get("tree_kind")) state.treeFilter.kind = params.get("tree_kind");
	  if (params.get("tree_priority")) state.treeFilter.priority = params.get("tree_priority");
	  if (params.get("tree_status")) state.treeFilter.status = params.get("tree_status");
	  if (params.get("worklog_q")) state.worklogFilter.q = params.get("worklog_q");
	  if (params.get("worklog_agent")) state.worklogFilter.agent = params.get("worklog_agent");
	  if (params.get("worklog_sort")) state.worklogSort = params.get("worklog_sort");
  bind();
  document.querySelectorAll("#page-nav li").forEach((li) => {
    li.classList.toggle("active", li.dataset.page === state.page);
  });
  loadProjects();
  startPolling();
})();

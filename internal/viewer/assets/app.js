"use strict";

const POLL_MS = 5000;
let state = {
  projectId: null,
  page: "dashboard",
  kanbanFilter: { priority: "", kind: "" },
  kanbanSort: "ts",
};
let pollTimer = null;

function $(id) { return document.getElementById(id); }
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

async function loadProjects(opts) {
  const background = Boolean(opts && opts.background);
  try {
    const projects = await getJSON("/api/projects");
    renderProjectList(projects);
    if (!state.projectId && projects.length > 0) {
      selectProject(projects[0].project_id);
    } else if (state.projectId) {
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
  if (state.kanbanSort !== "ts") params.set("sort", state.kanbanSort);
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
  if (!background) setLoading(page);
  try {
    switch (state.page) {
      case "dashboard": await renderDashboard(page); break;
      case "kanban":    await renderKanban(page); break;
      case "tree":      await renderTree(page); break;
      case "worklog":   await renderWorklog(page); break;
      case "insights":  await renderInsights(page); break;
      default:          page.innerHTML = ""; page.appendChild(el("div", { class: "state-empty", text: "Unknown page." }));
    }
  } catch (e) {
    setError(page, e);
  }
}

/* Dashboard */
async function renderDashboard(root) {
  const d = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/dashboard");
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

/* Kanban */
async function renderKanban(root) {
  const k = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/kanban");
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Kanban" }));

  // Filter bar
  const bar = el("div", { class: "kanban-bar" });
  const prioSel = el("select", { onchange: (e) => { state.kanbanFilter.priority = e.target.value; syncURL(); loadPage(); } });
  for (const v of ["", "P0", "P1", "P2", "P3"]) {
    const opt = el("option", { value: v, text: v ? "Priority " + v : "All priorities" });
    if (v === state.kanbanFilter.priority) opt.selected = true;
    prioSel.appendChild(opt);
  }
  const kindSel = el("select", { onchange: (e) => { state.kanbanFilter.kind = e.target.value; syncURL(); loadPage(); } });
  for (const v of ["", "plan", "issue", "task", "audit", "ops"]) {
    const opt = el("option", { value: v, text: v ? "Kind " + v : "All kinds" });
    if (v === state.kanbanFilter.kind) opt.selected = true;
    kindSel.appendChild(opt);
  }
  const sortSel = el("select", { onchange: (e) => { state.kanbanSort = e.target.value; syncURL(); loadPage(); } });
  for (const v of ["ts", "priority"]) {
    const opt = el("option", { value: v, text: "Sort by " + v });
    if (v === state.kanbanSort) opt.selected = true;
    sortSel.appendChild(opt);
  }
  bar.appendChild(prioSel); bar.appendChild(kindSel); bar.appendChild(sortSel);
  root.appendChild(bar);

  const cols = k.columns || [];
  if (cols.every((c) => (c.tickets || []).length === 0)) {
    root.appendChild(el("div", { class: "state-empty", text: "No tickets yet." }));
    return;
  }

  const board = el("div", { class: "kanban-board" });
  for (const col of cols) {
    const colEl = el("div", { class: "kanban-col" });
    const head = el("div", { class: "kanban-col-head" });
    head.appendChild(el("span", { class: "kanban-col-title", text: col.title }));

    let tickets = (col.tickets || []).filter((t) => {
      if (state.kanbanFilter.priority && (t.priority || "") !== state.kanbanFilter.priority) return false;
      if (state.kanbanFilter.kind && (t.kind || "") !== state.kanbanFilter.kind) return false;
      return true;
    });
    if (state.kanbanSort === "priority") {
      const rank = { "P0": 0, "P1": 1, "P2": 2, "P3": 3 };
      tickets = tickets.slice().sort((a, b) => (rank[a.priority] ?? 9) - (rank[b.priority] ?? 9));
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

function kanbanCard(t) {
  const card = el("div", { class: "kanban-card", onclick: () => openDrawer(t.ticket) });

  const top = el("div", { class: "kanban-card-top" });
  top.appendChild(el("span", { class: "mono kanban-card-id", text: t.ticket || "—" }));
  if (t.status) top.appendChild(el("span", { class: "pill " + t.status, text: t.status }));
  card.appendChild(top);

  const task = el("div", { class: "kanban-card-task", text: t.task || "" });
  card.appendChild(task);

  const badges = el("div", { class: "kanban-badges" });
  if (t.priority) badges.appendChild(el("span", { class: "badge badge-prio badge-prio-" + t.priority.toLowerCase(), text: t.priority }));
  if (t.kind && t.kind !== "task") badges.appendChild(el("span", { class: "badge", text: t.kind }));
  if (t.category) badges.appendChild(el("span", { class: "badge", text: t.category }));
  if (t.claimed_by) badges.appendChild(el("span", { class: "badge", text: "@" + t.claimed_by }));
  const blocked = (t.blocked_by || []).filter((s) => s);
  if (blocked.length > 0) badges.appendChild(el("span", { class: "badge badge-warn", text: "⛔ " + blocked.length }));
  if ((t.evidence || []).length > 0) badges.appendChild(el("span", { class: "badge badge-ok", text: "✓ ev" }));
  if (t.audit_result === "pass") badges.appendChild(el("span", { class: "badge badge-ok", text: "✓ audit" }));
  if (t.branch) badges.appendChild(el("span", { class: "badge badge-mono", text: t.branch }));
  if (badges.childNodes.length > 0) card.appendChild(badges);

  return card;
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
    body.appendChild(renderDrawerSummary(data.latest || {}, data.invalidated_via));
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
  head.appendChild(el("span", { class: "mono drawer-id", text: d.ticket || "—" }));
  const latest = d.latest || {};
  if (latest.status) head.appendChild(el("span", { class: "pill " + latest.status, text: latest.status }));
  return head;
}

function renderDrawerSummary(latest, invalidatedVia) {
  const wrap = el("div", { class: "drawer-summary" });
  if (latest.task) wrap.appendChild(el("div", { class: "drawer-task", text: latest.task }));
  const rows = [
    ["parent",     latest.parent_ticket],
    ["category",   latest.category],
    ["branch",     latest.branch],
    ["claimed_by", latest.claimed_by],
    ["agent",      latest.agent],
    ["role",       latest.role],
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
  if (dl.childNodes.length > 0) wrap.appendChild(dl);
  if (invalidatedVia) {
    wrap.appendChild(el("div", { class: "state-error", text: "This row is invalidated by n=" + invalidatedVia }));
  }
  return wrap;
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
    tb.appendChild(el("tr", null,
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
async function renderTree(root) {
  const t = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/tickets");
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

  // Build maps: id → row, and children of each parent (either workstream or ticket id).
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

  // A ticket that has a ticket-parent shouldn't ALSO appear at the top of its
  // workstream bucket — exclude such tickets from workstream listings.
  for (const [bucket, items] of workstreamBuckets) {
    workstreamBuckets.set(bucket, items.filter((it) => !byId.has(it.row.parent_ticket)));
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
      list.appendChild(renderTreeNode(item.row, childrenOf, byId, 0));
    }
    root.appendChild(list);
  }
}

function renderTreeNode(row, childrenOf, byId, depth) {
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
    wrap.appendChild(renderTreeNode(child.row, childrenOf, byId, depth + 1));
  }
  return wrap;
}

/* Worklog */
async function renderWorklog(root) {
  const w = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/worklog");
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Worklog" }));
  const rows = w.rows || [];
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
async function renderInsights(root) {
  const ins = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/insights");
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
function bind() {
  document.querySelectorAll("#page-nav li").forEach((li) => {
    li.addEventListener("click", () => selectPage(li.dataset.page));
  });
  document.querySelector("#drawer .close").addEventListener("click", () => $("drawer").classList.remove("open"));
}
function startPolling() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(() => loadProjects({ background: true }), POLL_MS);
}
(function init() {
  const params = new URLSearchParams(location.search);
  if (params.get("project")) state.projectId = params.get("project");
  if (params.get("page")) state.page = params.get("page");
  if (params.get("priority")) state.kanbanFilter.priority = params.get("priority");
  if (params.get("kind")) state.kanbanFilter.kind = params.get("kind");
  if (params.get("sort")) state.kanbanSort = params.get("sort");
  bind();
  document.querySelectorAll("#page-nav li").forEach((li) => {
    li.classList.toggle("active", li.dataset.page === state.page);
  });
  loadProjects();
  startPolling();
})();

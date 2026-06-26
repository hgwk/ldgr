
// ageHuman renders a millisecond age as a compact, human-friendly span.
function ageHuman(ms) {
  if (!ms || ms < 0) return "";
  const min = Math.round(ms / 60000);
  if (min < 60) return min + "m";
  const hr = Math.round(ms / 36e5);
  if (hr < 48) return hr + "h";
  return Math.round(hr / 24) + "d";
}

// insightTicketRow builds a clickable row that opens the ticket drawer.
function insightTicketRow(ticket, priority, status, task, metric) {
  const row = el("div", { class: "insight-row", onclick: () => ticket && openDrawer(ticket) });
  row.appendChild(el("span", { class: "mono", text: ticket || "—" }));
  if (priority) row.appendChild(el("span", { class: "badge badge-prio badge-prio-" + (priority || "").toLowerCase(), text: priority }));
  if (status) row.appendChild(el("span", { class: "pill " + status, text: status }));
  row.appendChild(el("span", { class: "insight-task", text: task || "" }));
  if (metric) row.appendChild(el("span", { class: "insight-metric muted", text: metric }));
  return row;
}

// insightPlainRow is a non-clickable detail row for rows that have no ticket id.
function insightPlainRow(text) {
  return el("div", { class: "insight-row insight-row-plain" }, el("span", { class: "muted", text: text }));
}

// INSIGHT_CATS defines each insight category: its summary-card severity tone
// and how to render one detail row. `sev` maps to a dot/border colour; `row`
// turns one API item into a detail row element.
const INSIGHT_CATS = [
  { key: "readyQueue", title: "Ready to start", sev: "good",
    row: (it) => insightTicketRow(ticketID(it), it.priority, it.status || it.state, ticketTitle(it)) },
  { key: "topBlockers", title: "Top blockers", sev: "high",
    row: (it) => insightTicketRow(ticketID(it), null, it.status, "blocks " + (it.dependents || []).length + " ticket(s)" + ((it.dependents || []).length ? ": " + it.dependents.join(", ") : "")) },
  { key: "staleInProgress", title: "Stale in_progress", sev: "warn",
    row: (it) => insightTicketRow(ticketID(it), null, it.status, ticketTitle(it), ageHuman(it.age_ms) + " since update") },
  { key: "closedWithoutWorklog", title: "Closed w/o worklog", sev: "warn",
    row: (it) => insightTicketRow(ticketID(it), it.priority, it.status || it.state, ticketTitle(it)) },
  { key: "worklogsWithoutTicket", title: "Orphan worklog", sev: "warn",
    row: (it) => insightPlainRow("worklog → ticket=" + (it.ticket || "?") + (it.task ? " · " + it.task : "")) },
  { key: "invalidated", title: "Invalidated rows", sev: "info",
    row: (it) => insightPlainRow("#" + it.n + " ← #" + it.via_n + " (" + it.kind + ")") },
];

async function renderInsights(root, background) {
  const ins = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/insights");
  if (shouldSkipRender("insights", ins, background)) return;
  root.innerHTML = "";

  const counts = {};
  let total = 0;
  for (const c of INSIGHT_CATS) { counts[c.key] = (ins[c.key] || []).length; total += counts[c.key]; }

  // Pick the active card: keep the user's last choice if it still has items,
  // otherwise fall back to the first non-empty category.
  let active = state.insightsCard;
  if (!active || counts[active] === 0) {
    const first = INSIGHT_CATS.find((c) => counts[c.key] > 0);
    active = first ? first.key : null;
  }

  const grid = el("div", { class: "insight-grid" });
  const detail = el("div", { class: "insight-detail" });
  const cardEls = {};

  function showDetail(c) {
    detail.innerHTML = "";
    detail.appendChild(el("div", { class: "section-heading" }, el("h3", { text: c.title + " · " + counts[c.key] })));
    const list = el("div", { class: "insight-list" });
    for (const it of (ins[c.key] || [])) list.appendChild(c.row(it));
    detail.appendChild(list);
  }

  for (const c of INSIGHT_CATS) {
    const n = counts[c.key];
    const card = el("div", { class: "insight-card sev-" + c.sev + (n === 0 ? " empty" : "") + (c.key === active ? " active" : "") });
    card.appendChild(el("span", { class: "insight-dot" + (n > 0 ? " on" : "") }));
    card.appendChild(el("div", { class: "insight-card-title", text: c.title }));
    card.appendChild(el("div", { class: "insight-card-count", text: String(n) }));
    if (n > 0) {
      card.addEventListener("click", () => {
        state.insightsCard = c.key;
        for (const k in cardEls) cardEls[k].classList.toggle("active", k === c.key);
        showDetail(c);
      });
    }
    cardEls[c.key] = card;
    grid.appendChild(card);
  }
  root.appendChild(grid);
  root.appendChild(detail);

  if (total === 0) {
    detail.appendChild(el("div", { class: "state-empty", text: "All clean — no insights at the moment." }));
  } else if (active) {
    showDetail(INSIGHT_CATS.find((c) => c.key === active));
  }
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
    closeBtn.addEventListener("click", closeDrawer);
  }
  document.addEventListener("click", (e) => {
    const drawer = $("drawer");
    if (!drawer || !drawer.classList.contains("open")) return;
    if (Date.now() - drawerOpenedAt < 100) return;
    if (drawer.contains(e.target)) return;
    closeDrawer();
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && $("drawer").classList.contains("open")) {
      closeDrawer();
    }
  });
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
  initSidebarToggle();
  const params = new URLSearchParams(location.search);
  if (params.get("project")) state.projectId = params.get("project");
  if (params.get("page")) state.page = normalizePage(params.get("page"));
  if (params.get("view")) state.ticketView = params.get("view") === "tree" ? "tree" : "kanban";
  if (params.get("priority")) state.kanbanFilter.priority = params.get("priority");
  if (params.get("kind")) state.kanbanFilter.kind = params.get("kind");
  if (params.get("status")) state.kanbanFilter.status = params.get("status");
  if (params.get("parent")) state.kanbanFilter.parent = params.get("parent");
  if (params.get("owner")) state.kanbanFilter.owner = params.get("owner");
  if (params.get("claim")) state.kanbanFilter.claim = params.get("claim");
  if (!state.kanbanFilter.owner && params.get("agent")) state.kanbanFilter.owner = params.get("agent");
  if (params.get("team")) state.kanbanFilter.team = params.get("team");
  if (params.get("blocked")) state.kanbanFilter.blocked = params.get("blocked");
  if (params.get("evidence")) state.kanbanFilter.evidence = params.get("evidence");
  if (params.get("sort")) state.kanbanSort = params.get("sort");
  if (["grid", "row", "column"].includes(params.get("layout"))) state.kanbanLayout = params.get("layout");
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

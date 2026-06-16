/* Kanban */
async function renderKanban(root, background) {
  const k = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/kanban");
  if (shouldSkipRender("kanban", k, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Tickets" }));
  appendTicketViewSwitch(root);

  const allTickets = [];
  for (const col of (k.columns || [])) for (const t of (col.tickets || [])) allTickets.push(t);
  const parents = uniqueSorted(allTickets.map((t) => t.parent_ticket || t.parent));
  const agents = uniqueSorted(allTickets.map((t) => t.claimed_by || t.owner || t.agent));
  const canonical = Array.isArray(k.grid) && k.grid.length > 0;
  const kindOptions = canonical
    ? ["", "epic", "plan", "issue", "task", "audit", "ops"]
    : ["", "plan", "issue", "task", "audit", "ops"];
  const stateOptions = canonical
    ? ["", "ready", "doing", "review", "rework", "backlog", "blocked", "done", "dropped"]
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
  appendEvidenceBadges(badges, t);
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

function appendEvidenceBadges(root, t) {
  const evidence = Array.isArray(t.evidence) ? t.evidence.filter((e) => typeof e === "string") : [];
  const kinds = uniqueSorted(evidence.map(testEvidenceKind).filter(Boolean));
  for (const kind of kinds.slice(0, 3)) {
    root.appendChild(el("span", {
      class: "badge " + (kind === "not_run" ? "badge-warn" : "badge-test"),
      text: kind,
      title: "test evidence",
    }));
  }
  if ((ticketState(t) === "review" || ticketState(t) === "done") && !kinds.some((k) => k !== "not_run")) {
    root.appendChild(el("span", { class: "badge badge-warn", text: "missing test" }));
  }
  if (evidence.some((e) => /^commit:/i.test(e.trim()))) {
    root.appendChild(el("span", { class: "badge badge-mono badge-test", text: "commit" }));
  } else if (evidence.some((e) => /^no_commit:/i.test(e.trim()))) {
    root.appendChild(el("span", { class: "badge badge-mono", text: "no_commit" }));
  }
}

function testEvidenceKind(evidence) {
  const v = evidence.trim().toLowerCase();
  if (!v) return "";
  if (v.startsWith("test:")) return v.slice(5).split(":")[0].trim();
  if (v.includes("playwright") || v.includes("browser")) return "browser";
  if (v.includes("smoke")) return "smoke";
  if (v.includes("e2e")) return "e2e";
  if (v.includes("integration")) return "integration";
  if (v.includes("typecheck") || v.includes("tsc")) return "typecheck";
  if (v.includes("lint") || v.includes("clippy")) return "lint";
  if (v.includes("go test") || v.includes("cargo test") || v.includes("npm test") || v.includes("pnpm test") || v.includes("yarn test") || v.includes("bun test")) return "unit";
  return "";
}

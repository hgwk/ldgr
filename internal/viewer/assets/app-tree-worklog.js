/* Tree */
async function renderTree(root, background) {
  const t = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/tickets");
  if (shouldSkipRender("tree", t, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Tickets" }));
  appendTicketViewSwitch(root);

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
  const treeParents = uniqueSorted(all.map((it) => it.row.parent_ticket || it.row.parent || it.parent));
  const canonical = all.some((it) => it.row.state || it.row.type || it.row.id);
  const kindOptions = uniqueSorted(all.map((it) => it.row.kind || it.row.type));
  const stateOptions = canonical
    ? ["", "ready", "doing", "review", "rework", "backlog", "blocked", "done", "dropped"]
    : ["", "open", "in_progress", "blocked", "audit_ready", "changes_requested", "done", "cancelled"];
  clearInvalidSelection(state.treeFilter, "kind", ["", ...kindOptions]);
  clearInvalidSelection(state.treeFilter, "status", stateOptions);
  const treeBar = el("div", { class: "kanban-bar" });
  treeBar.appendChild(selectControl(["", ...treeParents], state.treeFilter.parent, "All parents", (v) => { state.treeFilter.parent = v; syncURL(); loadPage(); }));
  treeBar.appendChild(selectControl(["", ...kindOptions].map(v => ({ value: v, text: v ? (canonical ? "Type " : "Kind ") + v : "" })), state.treeFilter.kind, canonical ? "All types" : "All kinds", (v) => { state.treeFilter.kind = v; syncURL(); loadPage(); }));
  treeBar.appendChild(selectControl(["", "P0", "P1", "P2", "P3"].map(v => ({ value: v, text: v ? "Priority " + v : "" })), state.treeFilter.priority, "All priorities", (v) => { state.treeFilter.priority = v; syncURL(); loadPage(); }));
  treeBar.appendChild(selectControl(stateOptions.map(v => ({ value: v, text: v ? (canonical ? "State " : "Status ") + v : "" })), state.treeFilter.status, canonical ? "All states" : "All statuses", (v) => { state.treeFilter.status = v; syncURL(); loadPage(); }));
  root.appendChild(treeBar);

  const byId = new Map();
  for (const item of all) byId.set(ticketID(item.row), item);
  const childrenOf = new Map();
  const workstreamBuckets = new Map();
  for (const item of all) {
    const p = item.row.parent_ticket || item.row.parent || item.parent || "—";
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
    workstreamBuckets.set(bucket, items.filter((it) => visible.has(ticketID(it.row)) && !byId.has(it.row.parent_ticket || it.row.parent)));
  }

  // Render each workstream bucket as a section drawn as a git-style commit
  // graph: every ticket is a coloured node on a lane, connected to its parent
  // and siblings by rails.
  const sortedBuckets = [...workstreamBuckets.keys()].sort();
  for (const parent of sortedBuckets) {
    const items = workstreamBuckets.get(parent);
    if (items.length === 0) continue;
    root.appendChild(el("div", { class: "section-heading" }, el("h3", { text: parent })));
    const list = el("div", { class: "tree-list tree-graph" });
    // Flatten this bucket's visible forest into DFS order, attaching the lane
    // metadata each row needs to draw its rails.
    const roots = [...items].sort((a, b) => (b.row.ts || "").localeCompare(a.row.ts || ""));
    const rows = [];
    roots.forEach((item, idx) => {
      flattenTree(item, 0, idx === roots.length - 1, idx === 0, [], childrenOf, visible, rows);
    });
    for (const r of rows) list.appendChild(renderGraphRow(r));
    root.appendChild(list);
  }
}

// flattenTree walks the visible subtree rooted at `item` in depth-first order,
// pushing one descriptor per node into `out`. ancMore[j] records whether the
// ancestor at depth j has a later sibling, so a vertical rail keeps running
// through this row at lane j.
function flattenTree(item, depth, isLast, isFirst, ancMore, childrenOf, visible, out) {
  const kids = (childrenOf.get(ticketID(item.row)) || [])
    .filter((k) => visible.has(ticketID(k.row)))
    .sort((a, b) => (b.row.ts || "").localeCompare(a.row.ts || ""));
  out.push({ row: item.row, depth, isLast, isFirst, ancMore: ancMore.slice(), hasKids: kids.length > 0 });
  const childAnc = ancMore.slice();
  childAnc[depth] = !isLast; // this node's rail continues iff it has a later sibling
  kids.forEach((k, idx) => {
    flattenTree(k, depth + 1, idx === kids.length - 1, false, childAnc, childrenOf, visible, out);
  });
}

const TREE_LANE_W = 18, TREE_ROW_H = 28, TREE_DOT_R = 4;

function renderGraphRow(r) {
  const d = r.depth, mid = TREE_ROW_H / 2;
  const width = (d + 1) * TREE_LANE_W;
  const cx = (i) => i * TREE_LANE_W + TREE_LANE_W / 2;
  const svgNS = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(svgNS, "svg");
  svg.setAttribute("class", "tree-graph-gutter");
  svg.setAttribute("width", width);
  svg.setAttribute("height", TREE_ROW_H);
  svg.setAttribute("viewBox", "0 0 " + width + " " + TREE_ROW_H);
  const line = (x1, y1, x2, y2) => {
    const l = document.createElementNS(svgNS, "line");
    l.setAttribute("x1", x1); l.setAttribute("y1", y1);
    l.setAttribute("x2", x2); l.setAttribute("y2", y2);
    l.setAttribute("class", "tree-graph-line");
    svg.appendChild(l);
  };
  // Rails of higher ancestors passing straight through this row.
  for (let j = 0; j < d - 1; j++) {
    if (r.ancMore[j]) line(cx(j), 0, cx(j), TREE_ROW_H);
  }
  if (d >= 1) {
    // Elbow from the parent lane into this node's lane.
    line(cx(d - 1), 0, cx(d - 1), mid);
    if (!r.isLast) line(cx(d - 1), mid, cx(d - 1), TREE_ROW_H);
    line(cx(d - 1), mid, cx(d), mid);
  } else {
    // Top-level node: rails connect roots down the trunk lane.
    if (!r.isFirst) line(cx(0), 0, cx(0), mid);
    if (!r.isLast) line(cx(0), mid, cx(0), TREE_ROW_H);
  }
  if (r.hasKids) line(cx(d), mid, cx(d), TREE_ROW_H); // down to first child
  const dot = document.createElementNS(svgNS, "circle");
  dot.setAttribute("cx", cx(d));
  dot.setAttribute("cy", mid);
  dot.setAttribute("r", TREE_DOT_R);
  dot.setAttribute("class", "tree-graph-dot status-" + (ticketState(r.row) || "open"));
  svg.appendChild(dot);

  const id = ticketID(r.row);
  const stateName = ticketState(r.row);
  const kind = r.row.kind || r.row.type || "";
  const title = r.row.task || r.row.title || "";
  const head = el("div", { class: "tree-node-head", onclick: () => openDrawer(id) });
  head.appendChild(el("span", { class: "mono", text: id }));
  if (r.row.priority) head.appendChild(el("span", { class: "badge badge-prio badge-prio-" + (r.row.priority || "").toLowerCase(), text: r.row.priority }));
  if (kind && kind !== "task") head.appendChild(el("span", { class: "badge", text: kind }));
  head.appendChild(el("span", { class: "pill " + stateName, text: stateName }));
  head.appendChild(el("span", { class: "tree-task", text: title }));
  head.appendChild(el("span", { class: "tree-ts muted", text: fmtTS(r.row.ts) }));

  const rowEl = el("div", { class: "tree-graph-row" });
  rowEl.appendChild(svg);
  rowEl.appendChild(head);
  return rowEl;
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
  if (state.treeFilter.parent && (item.row.parent_ticket || item.row.parent || item.parent || "") !== state.treeFilter.parent) return false;
  if (state.treeFilter.kind && (item.row.kind || item.row.type || "") !== state.treeFilter.kind) return false;
  if (state.treeFilter.priority && (item.row.priority || "") !== state.treeFilter.priority) return false;
  if (state.treeFilter.status && ticketState(item.row) !== state.treeFilter.status) return false;
  return true;
}

function markVisibleWithAncestors(item, byId, visible) {
  let cur = item;
  while (cur && cur.row && ticketID(cur.row) && !visible.has(ticketID(cur.row))) {
    visible.add(ticketID(cur.row));
    cur = byId.get(cur.row.parent_ticket || cur.row.parent);
  }
}

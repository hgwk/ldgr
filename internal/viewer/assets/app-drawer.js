async function openDrawer(ticketId) {
  const drawer = $("drawer");
  const body = $("drawer-body");
  drawerOpenedAt = Date.now();
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

function closeDrawer() {
  const drawer = $("drawer");
  drawer.classList.remove("open");
  drawer.setAttribute("aria-hidden", "true");
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
    ["team",        latest.team],
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
      el("td", null, ticketState(r) ? el("span", { class: "pill " + ticketState(r), text: ticketState(r) }) : document.createTextNode("")),
      el("td", { text: r.role || (r.event && r.event.role) || "" }),
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

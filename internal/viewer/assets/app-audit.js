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
  const items = (q && q.items) || [];

  if (items.length === 0) {
    root.appendChild(el("div", { class: "state-empty audit-empty", text: "No tickets are waiting for audit. Nice." }));
    return;
  }

  // Auditor's worklist: longest-waiting at the top (FIFO + staleness), with the
  // no-evidence tickets (which can't pass audit as-is) surfaced first of all.
  const ageMs = (it) => { const t = it.waiting_since && new Date(it.waiting_since).getTime(); return t && !isNaN(t) ? Date.now() - t : -1; };
  const sorted = items.slice().sort((a, b) => {
    const ea = a.has_evidence ? 1 : 0, eb = b.has_evidence ? 1 : 0;
    if (ea !== eb) return ea - eb;           // missing evidence first
    return ageMs(b) - ageMs(a);              // then oldest-waiting first
  });
  const missing = items.filter((it) => !it.has_evidence).length;
  const stale = items.filter((it) => ageMs(it) >= STALE_AUDIT_MS).length;
  const oldest = sorted.length ? describeAge(sorted.reduce((m, it) => ageMs(it) > ageMs(m) ? it : m).waiting_since).replace("waiting ", "") : "";
  const summary = el("div", { class: "muted audit-subtitle" });
  summary.appendChild(el("span", { text: items.length + " " + (items.length === 1 ? "ticket" : "tickets") + " waiting" }));
  if (missing) summary.appendChild(el("span", { class: "audit-summary-warn", text: " · " + missing + " missing evidence" }));
  if (stale) summary.appendChild(el("span", { class: "audit-summary-warn", text: " · " + stale + " stale (>2d)" }));
  if (oldest) summary.appendChild(el("span", { text: " · oldest " + oldest }));
  root.appendChild(summary);

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
  for (const it of sorted) {
    const cls = "audit-row"
      + (!it.has_evidence ? " audit-row-noev" : "")
      + (ageMs(it) >= STALE_AUDIT_MS ? " audit-row-stale" : "");
    const tr = el("tr", { class: cls, onclick: () => openDrawer(it.ticket_id) });
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


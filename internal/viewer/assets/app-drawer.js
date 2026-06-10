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

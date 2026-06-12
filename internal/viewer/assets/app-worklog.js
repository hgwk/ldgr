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
  // Render as a day-grouped activity timeline. Cap the rendered count, but
  // surface the truncation instead of silently dropping rows.
  const MAX = 150;
  const truncated = rows.length > MAX;
  const shown = rows.slice(0, MAX);
  const todayKey = new Date().toISOString().substring(0, 10);
  const yesterdayKey = new Date(Date.now() - 86400000).toISOString().substring(0, 10);
  const dayCounts = {};
  for (const r of shown) {
    const k = (r.ts || "").substring(0, 10) || "—";
    dayCounts[k] = (dayCounts[k] || 0) + 1;
  }

  const feed = el("div", { class: "wl-feed" });
  let curDay = null, entries = null;
  for (const r of shown) {
    const dayKey = (r.ts || "").substring(0, 10) || "—";
    if (dayKey !== curDay) {
      curDay = dayKey;
      const label = dayKey === todayKey ? "Today" : dayKey === yesterdayKey ? "Yesterday" : dayKey;
      feed.appendChild(el("div", { class: "wl-day-head" },
        el("span", { text: label }),
        el("span", { class: "muted", text: " · " + dayCounts[dayKey] })));
      entries = el("div", { class: "wl-entries" });
      feed.appendChild(entries);
    }
    entries.appendChild(renderWorklogEntry(r));
  }
  root.appendChild(feed);
  if (truncated) {
    root.appendChild(el("div", { class: "wl-more muted", text: "Showing " + MAX + " of " + rows.length + " entries — narrow with search or the agent filter." }));
  }
}

// hhmm renders the UTC HH:MM portion of a timestamp (matches fmtTS's clock).
function hhmm(ts) {
  if (!ts) return "—";
  const d = new Date(ts);
  if (isNaN(d.getTime())) return "";
  return d.toISOString().substring(11, 16);
}

// agentHue derives a stable hue (0-359) from an agent name so each agent gets
// a consistent chip colour without a fixed palette.
function agentHue(name) {
  let h = 0;
  const s = name || "";
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h % 360;
}

function renderWorklogEntry(r) {
  const entry = el("div", { class: "wl-entry" + (r.ticket ? " wl-clickable" : "") });
  if (r.ticket) entry.addEventListener("click", () => openDrawer(r.ticket));
  entry.appendChild(el("span", { class: "wl-dot" }));
  const body = el("div", { class: "wl-body" });
  const meta = el("div", { class: "wl-meta" });
  meta.appendChild(el("span", { class: "wl-time", text: hhmm(r.ts), title: fmtTS(r.ts) }));
  if (r.agent) {
    const hue = agentHue(r.agent);
    const chip = el("span", { class: "wl-agent", style: "color: oklch(0.55 0.13 " + hue + "); border-color: oklch(0.7 0.1 " + hue + ")", text: r.agent });
    chip.addEventListener("click", (e) => { e.stopPropagation(); state.worklogFilter.agent = r.agent; syncURL(); loadPage(); });
    meta.appendChild(chip);
  }
  if (r.ticket) meta.appendChild(el("span", { class: "mono wl-ticket", text: r.ticket }));
  body.appendChild(meta);
  const text = el("div", { class: "wl-text" });
  text.appendChild(el("span", { class: "wl-task", text: r.task || "" }));
  if (r.result) {
    text.appendChild(el("span", { class: "wl-arrow muted", text: " → " }));
    text.appendChild(el("span", { class: "wl-result", text: r.result }));
  }
  body.appendChild(text);
  entry.appendChild(body);
  return entry;
}

/* Insights */

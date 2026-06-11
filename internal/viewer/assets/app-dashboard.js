"use strict";

async function renderDashboard(root, background) {
  const d = await getJSON("/api/projects/" + encodeURIComponent(state.projectId) + "/dashboard");
  if (shouldSkipRender("dashboard", d, background)) return;
  root.innerHTML = "";
  root.appendChild(el("div", { class: "page-title", text: "Dashboard" }));

  const metrics = el("div", { class: "dashboard-metrics" });
  metrics.appendChild(metricTile("Progress", (d.progress && d.progress.percent || 0) + "%", progressDetail(d.progress)));
  metrics.appendChild(metricTile("Active", String(d.progress && d.progress.active || 0), "done " + (d.progress && d.progress.done || 0)));
  metrics.appendChild(metricTile("Review", String(d.audit && d.audit.audit_ready || 0), "rework " + (d.audit && d.audit.changes_requested || 0)));
  metrics.appendChild(metricTile("Blocked claims", String(d.stale_claims && d.stale_claims.expired || 0), "near " + (d.stale_claims && d.stale_claims.near_expiring || 0)));
  metrics.appendChild(metricTile("Cycle", formatLatencyBucket(d.lifecycle && d.lifecycle.median_cycle_hours || 0), "p90 " + formatLatencyBucket(d.lifecycle && d.lifecycle.p90_cycle_hours || 0)));
  metrics.appendChild(metricTile("Agents", String((d.active_agents && d.active_agents.agents || []).length), "last 24h"));
  root.appendChild(metrics);

  const split = el("div", { class: "dashboard-split" });
  split.appendChild(parentPanel(d.parents || []));
  split.appendChild(recentPanel(d.recent || []));
  root.appendChild(split);
}

function metricTile(label, value, detail) {
  return el("div", { class: "metric" },
    el("div", { class: "metric-label", text: label }),
    el("div", { class: "metric-value", text: value }),
    el("div", { class: "metric-detail", text: detail || "" }));
}

function progressDetail(p) {
  if (!p) return "";
  return p.done + " done · " + p.active + " active";
}

function parentPanel(parents) {
  const panel = el("section", { class: "dashboard-panel" });
  panel.appendChild(el("div", { class: "section-heading" }, el("h3", { text: "Parents" })));
  const list = el("div", { class: "dashboard-list" });
  for (const p of parents.slice(0, 12)) {
    const row = el("div", { class: "dashboard-row" });
    row.appendChild(el("span", { class: "mono", text: p.parent || "—" }));
    row.appendChild(el("span", { text: (p.percent || 0) + "%" }));
    row.appendChild(el("span", { class: "muted", text: p.active + " active" }));
    list.appendChild(row);
  }
  if (parents.length === 0) list.appendChild(el("div", { class: "state-empty", text: "No parent progress yet." }));
  panel.appendChild(list);
  return panel;
}

function recentPanel(items) {
  const panel = el("section", { class: "dashboard-panel" });
  panel.appendChild(el("div", { class: "section-heading" }, el("h3", { text: "Recent" })));
  const list = el("div", { class: "dashboard-list" });
  for (const it of items.slice(0, 12)) {
    const row = el("div", { class: "dashboard-row dashboard-row-clickable", onclick: () => it.ticket && openDrawer(it.ticket) });
    row.appendChild(el("span", { class: "mono", text: it.ticket || it.kind || "—" }));
    row.appendChild(el("span", { class: "muted", text: fmtTS(it.ts) }));
    row.appendChild(el("span", { text: it.task || it.result || "" }));
    list.appendChild(row);
  }
  if (items.length === 0) list.appendChild(el("div", { class: "state-empty", text: "No recent activity." }));
  panel.appendChild(list);
  return panel;
}

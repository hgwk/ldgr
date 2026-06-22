"use strict";

function coordinationRail(coordination) {
  const rail = el("aside", { class: "coord-rail" });
  rail.appendChild(el("div", { class: "coord-title", text: "Coordination" }));
  rail.appendChild(coordinationSection("Conflicts", coordinationConflictRows(coordination), "No conflicts."));
  rail.appendChild(coordinationSection("Active Claims", coordinationClaimRows(coordination), "No active claims."));
  rail.appendChild(coordinationSection("Recent Notes", coordinationNoteRows(coordination), "No shared notes."));
  return rail;
}

function coordinationSection(title, rows, emptyText) {
  const section = el("section", { class: "coord-section" });
  section.appendChild(el("div", { class: "coord-section-title", text: title }));
  const list = el("div", { class: "coord-list" });
  if (rows.length === 0) {
    list.appendChild(el("div", { class: "coord-empty", text: emptyText }));
  } else {
    for (const row of rows) list.appendChild(row);
  }
  section.appendChild(list);
  return section;
}

function coordinationConflictRows(coordination) {
  return (coordination && coordination.conflicts || []).slice(0, 8).map((c) => {
    const row = el("button", { class: "coord-row coord-row-alert", type: "button", onclick: () => c.second && c.second.ticket && openDrawer(c.second.ticket) });
    row.appendChild(el("span", { class: "coord-main", text: c.resource || "resource" }));
    row.appendChild(el("span", { class: "coord-sub", text: claimText(c.first) + " ↔ " + claimText(c.second) }));
    return row;
  });
}

function coordinationClaimRows(coordination) {
  return (coordination && coordination.claims || []).slice(0, 10).map((claim) => {
    const cls = "coord-row" + (claim.expired ? " coord-row-stale" : "");
    const row = el("button", { class: cls, type: "button", onclick: () => claim.ticket && openDrawer(claim.ticket) });
    row.appendChild(el("span", { class: "coord-main", text: (claim.resources || []).join(", ") || claim.id || "claim" }));
    row.appendChild(el("span", { class: "coord-sub", text: claimText(claim) + " · " + (claim.expires_in || "") }));
    return row;
  });
}

function coordinationNoteRows(coordination) {
  return (coordination && coordination.notes || []).slice(0, 8).map((note) => {
    const firstTicket = note.ticket || (note.tickets && note.tickets[0]) || "";
    const row = el("button", { class: "coord-row", type: "button", onclick: () => firstTicket && openDrawer(firstTicket) });
    row.appendChild(el("span", { class: "coord-main", text: (note.kind || "note") + (note.scope ? " · " + note.scope : "") }));
    row.appendChild(el("span", { class: "coord-sub", text: note.summary || "" }));
    return row;
  });
}

function claimText(claim) {
  if (!claim) return "—";
  const parts = [];
  if (claim.ticket) parts.push(claim.ticket);
  if (claim.lane) parts.push(claim.lane);
  if (claim.owner) parts.push("@" + claim.owner);
  return parts.join(" / ") || claim.id || "claim";
}

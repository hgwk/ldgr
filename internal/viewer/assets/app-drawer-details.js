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

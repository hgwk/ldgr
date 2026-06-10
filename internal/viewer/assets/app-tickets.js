"use strict";

async function renderTickets(root, background) {
  if (state.ticketView === "tree") {
    await renderTree(root, background);
    return;
  }
  await renderKanban(root, background);
}

function appendTicketViewSwitch(root) {
  const title = root.querySelector(".page-title");
  if (title) title.textContent = "Tickets";
  const bar = el("div", { class: "kanban-bar" });
  bar.appendChild(ticketViewSwitch());
  if (title && title.nextSibling) {
    root.insertBefore(bar, title.nextSibling);
    return;
  }
  root.insertBefore(bar, root.firstChild);
}

function ticketViewSwitch() {
  const group = el("div", { class: "view-switch", role: "group", "aria-label": "Ticket view" });
  for (const view of ["kanban", "tree"]) {
    const active = state.ticketView === view;
    group.appendChild(el("button", {
      type: "button",
      class: active ? "active" : "",
      "aria-pressed": active ? "true" : "false",
      text: view === "kanban" ? "Kanban" : "Tree",
      onclick: () => {
        state.ticketView = view;
        syncURL();
        loadPage();
      },
    }));
  }
  return group;
}

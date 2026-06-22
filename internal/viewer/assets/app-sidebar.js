"use strict";

function applySidebarCollapsed(collapsed) {
  document.body.classList.toggle("sidebar-collapsed", collapsed);
  const btn = document.getElementById("sidebar-toggle");
  if (!btn) return;
  btn.textContent = "";
  btn.appendChild(icon("panel"));
  btn.setAttribute("aria-pressed", collapsed ? "true" : "false");
  btn.setAttribute("aria-label", collapsed ? "Open sidebar" : "Close sidebar");
  btn.setAttribute("title", collapsed ? "Open sidebar" : "Close sidebar");
}

function initSidebarToggle() {
  let collapsed = false;
  try { collapsed = localStorage.getItem("ldgr.sidebar") === "collapsed"; } catch (_) {}
  applySidebarCollapsed(collapsed);

  const btn = document.getElementById("sidebar-toggle");
  if (!btn) return;
  btn.addEventListener("click", () => {
    const next = !document.body.classList.contains("sidebar-collapsed");
    try { localStorage.setItem("ldgr.sidebar", next ? "collapsed" : "open"); } catch (_) {}
    applySidebarCollapsed(next);
  });
}

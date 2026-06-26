/* Geeky animated background: deep-sea aurora. A dark blue-black gradient with
   soft god-rays drifting down from a light source above the top edge, plus a few
   faint floating particles. Rendered on #bg-fx behind all content; the sidebar
   and cards/panels stay opaque, so it shows through the main area's empty space.
   Dark theme only — in light theme the canvas clears to the plain page bg. */
(function () {
  "use strict";
  const canvas = document.getElementById("bg-fx");
  if (!canvas) return;
  const ctx = canvas.getContext("2d", { alpha: true });
  if (!ctx) return;

  const reduceMotion = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const FRAME_MS = 1000 / 30;

  // God-rays fanning out from a source above the top edge (upper-left, like the
  // reference). Each sways and pulses on its own phase for a living shimmer.
  // Kept deliberately faint — the effect should be almost subliminal, a slow
  // shimmer you only notice if you look for it, never competing with content.
  const RAYS = [
    { ang: -0.12, w: 46, a: 0.045, sway: 0.05, sp: 0.00018, ph: 0.0 },
    { ang: 0.02,  w: 34, a: 0.055, sway: 0.04, sp: 0.00024, ph: 1.3 },
    { ang: 0.16,  w: 60, a: 0.038, sway: 0.06, sp: 0.00015, ph: 2.1 },
    { ang: 0.30,  w: 30, a: 0.050, sway: 0.05, sp: 0.00027, ph: 3.4 },
    { ang: 0.46,  w: 52, a: 0.032, sway: 0.07, sp: 0.00013, ph: 4.7 },
    { ang: 0.60,  w: 26, a: 0.040, sway: 0.05, sp: 0.00021, ph: 5.5 },
  ];
  let particles = [];
  let W = 0, H = 0;

  function isDark() { return document.documentElement.classList.contains("dark"); }

  function resize() {
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    W = window.innerWidth; H = window.innerHeight;
    canvas.width = Math.floor(W * dpr);
    canvas.height = Math.floor(H * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const n = Math.round(W / 24);
    particles = [];
    for (let i = 0; i < n; i++) {
      particles.push({
        x: Math.random() * W, y: Math.random() * H,
        r: 0.5 + Math.random() * 1.4,
        vy: -(0.05 + Math.random() * 0.18),
        vx: (Math.random() - 0.5) * 0.12,
        a: 0.04 + Math.random() * 0.10,
        tw: Math.random() * 6.28,
      });
    }
  }

  function paintBg() {
    // Transparent base — the app's own dark bg shows through. We only lay a very
    // faint blue wash on top so the page reads as "barely tinted", not repainted.
    ctx.clearRect(0, 0, W, H);
    const g = ctx.createLinearGradient(0, 0, 0, H);
    g.addColorStop(0, "rgba(26, 66, 112, 0.10)");
    g.addColorStop(0.5, "rgba(14, 40, 72, 0.04)");
    g.addColorStop(1, "rgba(2, 7, 14, 0)");
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, W, H);
    const src = { x: W * 0.42, y: -H * 0.12 };
    const glow = ctx.createRadialGradient(src.x, src.y, 0, src.x, src.y, H * 0.9);
    glow.addColorStop(0, "rgba(70, 140, 200, 0.10)");
    glow.addColorStop(0.4, "rgba(40, 90, 150, 0.035)");
    glow.addColorStop(1, "rgba(0, 0, 0, 0)");
    ctx.fillStyle = glow;
    ctx.fillRect(0, 0, W, H);
  }

  function drawRays(t) {
    const src = { x: W * 0.42, y: -H * 0.12 };
    const len = H * 1.5;
    ctx.save();
    ctx.globalCompositeOperation = "lighter";
    if (ctx.filter !== undefined) ctx.filter = "blur(22px)";
    for (const r of RAYS) {
      const ang = r.ang + Math.sin(t * r.sp + r.ph) * r.sway;
      const alpha = r.a * (0.6 + 0.4 * Math.sin(t * r.sp * 1.7 + r.ph));
      ctx.save();
      ctx.translate(src.x, src.y);
      ctx.rotate(ang);
      const grad = ctx.createLinearGradient(0, 0, 0, len);
      grad.addColorStop(0, "rgba(150, 200, 240, " + alpha.toFixed(3) + ")");
      grad.addColorStop(0.55, "rgba(110, 170, 225, " + (alpha * 0.5).toFixed(3) + ")");
      grad.addColorStop(1, "rgba(110, 170, 225, 0)");
      ctx.fillStyle = grad;
      ctx.fillRect(-r.w, 0, r.w * 2, len);
      ctx.restore();
    }
    ctx.restore();
  }

  function drawParticles(t) {
    ctx.save();
    ctx.globalCompositeOperation = "lighter";
    for (const p of particles) {
      p.y += p.vy; p.x += p.vx;
      if (p.y < -4) { p.y = H + 4; p.x = Math.random() * W; }
      if (p.x < -4) p.x = W + 4; else if (p.x > W + 4) p.x = -4;
      const a = p.a * (0.5 + 0.5 * Math.sin(t * 0.001 + p.tw));
      ctx.beginPath();
      ctx.arc(p.x, p.y, p.r, 0, 6.2832);
      ctx.fillStyle = "rgba(170, 205, 235, " + a.toFixed(3) + ")";
      ctx.fill();
    }
    ctx.restore();
  }

  function frame(t) {
    if (!isDark()) { ctx.clearRect(0, 0, W, H); return; }
    paintBg();
    drawRays(t);
    drawParticles(t);
  }

  let last = 0, raf = 0, running = false;
  function loop(now) {
    raf = requestAnimationFrame(loop);
    if (now - last < FRAME_MS) return;
    last = now;
    frame(now);
  }
  function start() { if (!running) { running = true; raf = requestAnimationFrame(loop); } }
  function stop() { running = false; cancelAnimationFrame(raf); }

  resize();
  let rt = 0;
  window.addEventListener("resize", function () { clearTimeout(rt); rt = setTimeout(resize, 150); });

  if (reduceMotion) { frame(0); return; }
  document.addEventListener("visibilitychange", function () { if (document.hidden) stop(); else start(); });
  start();
})();

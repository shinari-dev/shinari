// SPDX-License-Identifier: Apache-2.0
// © The Shinari Authors

// Hero scene: a kaiju under test.
//
// A city skyline of lit buildings is the system at steady state. A kaiju
// silhouette looms behind it and walks the horizon — as it passes, buildings
// take damage (lights go dark, towers collapse, windows flicker) and then
// REBUILD once it moves on. That is Shinari's whole loop: inject a fault,
// watch the system survive and recover.
//
// Damage comes in kinds, mapped to the engine's Effect split:
//   collapse  a tower loses its top      (outage, ember)
//   flicker   windows stutter then drop  (outage, ember)
//   blackout  the block dims but stands  (degradation, amber)
//
// Rendered as ember-on-near-black silhouette to sit inside the dark, cinematic
// brand aesthetic — deliberately not pixel art.
(function () {
  "use strict";

  var canvas = document.getElementById("hero-grid");
  if (!canvas || !canvas.getContext) return;
  var ctx = canvas.getContext("2d");

  var EMBER = [255, 79, 43];
  var EMBER_SOFT = [255, 140, 96];
  var HOT = [255, 226, 200];
  var AMBER = [255, 180, 84];
  var BUILDING = [18, 20, 28];

  var dpr = Math.max(1, window.devicePixelRatio || 1);
  var buildings = [];
  var stomps = []; // ground-impact ripples from footfalls
  var lastStep = 0;
  var w = 0;
  var h = 0;
  var seed = 11;

  // kaiju sprite sheet: 4 frames [walk_a, walk_b, charge, fire], left edge order
  var sprite = new Image();
  var spriteReady = false;
  var CELL_W = 581;
  var CELL_H = 555;
  var GUTTER = 8; // transparent px between frames (stops white edge-bleed when scaling)
  // sheet order: 5 idle-pulse frames, then walk_a, walk_b, charge, fire
  var FRAMES = { idle: [0, 1, 2, 3, 4], walkA: 5, walkB: 6, charge: 7, fire: 8 };
  var KAIJU_H = 0.34; // sprite height as a fraction of the hero height
  // mouth (beam origin) as a fraction of the fire cell
  var MOUTH_FX = 0.804;
  var MOUTH_FY = 0.187;

  // kaiju behaviour: walk in -> charge -> fire ember beam -> seek next target
  var kaiju = { x: 0, state: "walk", t: 0, walkT: 0, target: -1, lastHit: -1, dir: 1 };
  var prevNow = 0;
  var WALK_SPEED = 42; // px/s
  var MIN_WALK_MS = 1500; // stride this long, then pause to idle
  var IDLE_MS = 1700; // breathe in place (idle pulse) before walking on
  var CHARGE_MS = 850;
  var FIRE_MS = 760;
  var FIRE_MIN = 46; // beam only fires when a fresh target sits in this gap
  var FIRE_MAX = 168;

  function rnd() {
    seed = (seed * 1103515245 + 12345) & 0x7fffffff;
    return seed / 0x7fffffff;
  }
  function rgba(c, a) {
    return "rgba(" + c[0] + "," + c[1] + "," + c[2] + "," + (a < 0 ? 0 : a > 1 ? 1 : a).toFixed(3) + ")";
  }
  function mix(a, b, t) {
    return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t];
  }
  function clamp01(v) {
    return v < 0 ? 0 : v > 1 ? 1 : v;
  }

  function build() {
    var rect = canvas.getBoundingClientRect();
    w = rect.width;
    h = rect.height;
    canvas.width = Math.round(w * dpr);
    canvas.height = Math.round(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    buildings = [];
    var x = -10;
    while (x < w + 10) {
      var bw = 11 + rnd() * 20;
      var bh = h * (0.05 + rnd() * 0.15);
      var b = { x: x, w: bw, h: bh, dmg: 0, type: null, windows: [] };
      var pad = 3;
      for (var wy = pad; wy < bh - pad; wy += 5) {
        for (var wx = pad; wx < bw - pad - 1; wx += 5) {
          b.windows.push({ x: wx, y: wy, on: rnd() > 0.34, ph: rnd() * 6.28 });
        }
      }
      buildings.push(b);
      x += bw + 4 + rnd() * 9;
    }
  }

  // ---- kaiju sprite ---------------------------------------------------------
  //
  // Drawn from a 4-frame sprite sheet (walk_a, walk_b, charge, fire), bottom
  // aligned and scaled to KAIJU_H of the hero. Mirrored when facing left.

  function spriteScale() {
    return (h * KAIJU_H) / CELL_H;
  }

  function drawKaiju(cx, baseY, frameIndex, walking, now, dir) {
    if (!spriteReady) return;
    var scale = spriteScale();
    var dw = CELL_W * scale;
    var dh = CELL_H * scale;
    var bob = walking && Math.floor(now / 200) % 2 ? scale * 6 : 0; // gentle stride bob
    var dx = Math.round(cx - dw / 2);
    var dy = Math.round(baseY - dh - bob);

    ctx.save();
    if (dir < 0) {
      ctx.translate(Math.round(cx) * 2, 0); // mirror about the sprite centre
      ctx.scale(-1, 1);
    }
    ctx.drawImage(sprite, frameIndex * (CELL_W + GUTTER), 0, CELL_W, CELL_H, dx, dy, dw, dh);
    ctx.restore();
  }

  function drawBeam(mx, my, b, now, unit) {
    // angle the beam down onto the target's roofline so it visibly strikes.
    // widths scale with the kaiju (unit = its on-screen height) so the beam
    // stays proportionally thick no matter the kaiju size.
    var ex = b.x + b.w / 2;
    var ey = h - b.h * 0.85;
    if (ey < my) ey = my; // never aim upward
    var flick = 0.74 + 0.26 * Math.sin(now * 0.05);
    ctx.lineCap = "round";
    ctx.shadowColor = rgba(EMBER, 0.9);
    ctx.shadowBlur = unit * 0.09;
    var layers = [
      [EMBER, 0.4, 0.3],
      [EMBER_SOFT, 0.9, 0.17],
      [HOT, 1, 0.08],
    ];
    for (var i = 0; i < layers.length; i++) {
      if (i === 2) ctx.shadowBlur = unit * 0.05;
      ctx.strokeStyle = rgba(layers[i][0], layers[i][1] * flick);
      ctx.lineWidth = Math.max(1.5, unit * layers[i][2]);
      ctx.beginPath();
      ctx.moveTo(mx, my);
      ctx.lineTo(ex, ey);
      ctx.stroke();
    }
    // impact burst at the roofline
    ctx.fillStyle = rgba(HOT, 0.9 * flick);
    ctx.beginPath();
    ctx.arc(ex, ey, unit * 0.05 + 2 * Math.sin(now * 0.06), 0, Math.PI * 2);
    ctx.fill();
    ctx.shadowColor = rgba(EMBER, 0.7);
    ctx.shadowBlur = unit * 0.08;
    ctx.fillStyle = rgba(EMBER, 0.5 * flick);
    ctx.beginPath();
    ctx.arc(ex, ey, unit * 0.09 + 3 * Math.sin(now * 0.06), 0, Math.PI * 2);
    ctx.fill();
    ctx.shadowBlur = 0;
    ctx.lineCap = "butt";
  }

  // ---- buildings ------------------------------------------------------------

  function drawBuilding(b, now) {
    var collapse = b.type === "collapse" ? b.dmg : 0;
    var bh = b.h * (1 - collapse * 0.72);
    var top = h - bh;
    var origTop = h - b.h;

    ctx.fillStyle = rgba(BUILDING, 0.95);
    ctx.fillRect(b.x, top, b.w, bh);

    // top edge — glows ember while collapsing
    var edge = 0.16 + (collapse > 0 ? 0.5 * Math.sin(now * 0.02 + b.x) * collapse : 0);
    ctx.fillStyle = rgba(EMBER, clamp01(edge));
    ctx.fillRect(b.x, top, b.w, 1.5);

    for (var i = 0; i < b.windows.length; i++) {
      var win = b.windows[i];
      var ay = origTop + win.y;
      if (ay < top) continue; // sheared off by collapse

      var lit = win.on;
      var a = lit ? 0.5 + 0.4 * Math.sin(now * 0.002 + win.ph) : 0.08;
      var col = EMBER;

      if (b.dmg > 0) {
        if (b.type === "blackout") {
          a *= 1 - b.dmg;
          col = mix(EMBER, AMBER, b.dmg);
        } else if (b.type === "flicker") {
          a *= (0.45 + 0.55 * Math.sin(now * 0.05 + win.ph)) * (1 - b.dmg * 0.55);
        } else if (b.type === "collapse") {
          a *= 1 - b.dmg * 0.7;
        }
      }
      ctx.fillStyle = rgba(col, clamp01(a) * (lit ? 0.9 : 0.22));
      ctx.fillRect(b.x + win.x, ay, 2, 2);
    }
  }

  // ---- frame ----------------------------------------------------------------

  function frame(now, animate) {
    ctx.clearRect(0, 0, w, h);

    var scale = spriteScale();
    var dw = CELL_W * scale;
    var dh = CELL_H * scale;
    var baseY = h + 2; // feet at the skyline base
    var dt = animate ? Math.min(70, now - prevNow) : 0;
    prevNow = now;

    var mouthX = kaiju.x + kaiju.dir * (MOUTH_FX - 0.5) * dw;
    var mouthY = baseY - dh + MOUTH_FY * dh;
    var firing = false;

    if (animate) {
      if (kaiju.state === "walk") {
        kaiju.x += (WALK_SPEED * kaiju.dir * dt) / 1000;
        kaiju.walkT += dt;
        // a footfall ripple on the ground every step
        if (now - lastStep > 360) {
          lastStep = now;
          stomps.push({ x: kaiju.x, t0: now });
        }
        // stay in the right half so it never covers the headline / CTA buttons
        var leftB = w * 0.54;
        var rightB = w - dw / 2 - 6;
        if (kaiju.x > rightB) {
          kaiju.x = rightB;
          kaiju.dir = -1;
          kaiju.walkT = 0;
          kaiju.lastHit = -1;
        } else if (kaiju.x < leftB) {
          kaiju.x = leftB;
          kaiju.dir = 1;
          kaiju.walkT = 0;
          kaiju.lastHit = -1;
        }
        // after striding a while, stop and breathe (idle pulse)
        if (kaiju.walkT > MIN_WALK_MS) {
          kaiju.state = "idle";
          kaiju.t = 0;
        }
      } else if (kaiju.state === "idle") {
        kaiju.t += dt;
        // while idling, lock onto a fresh building ahead and attack
        var best = -1,
          bestd = 1e9;
        for (var i = 0; i < buildings.length; i++) {
          if (i === kaiju.lastHit) continue;
          var ahead = (buildings[i].x + buildings[i].w / 2 - mouthX) * kaiju.dir;
          if (ahead > FIRE_MIN && ahead < FIRE_MAX && ahead < bestd) {
            bestd = ahead;
            best = i;
          }
        }
        if (best >= 0) {
          kaiju.state = "charge";
          kaiju.t = 0;
          kaiju.target = best;
        } else if (kaiju.t >= IDLE_MS) {
          kaiju.state = "walk";
          kaiju.walkT = 0;
        }
      } else if (kaiju.state === "charge") {
        kaiju.t += dt;
        if (kaiju.t >= CHARGE_MS) {
          kaiju.state = "fire";
          kaiju.t = 0;
        }
      } else if (kaiju.state === "fire") {
        kaiju.t += dt;
        firing = true;
        if (kaiju.t >= FIRE_MS) {
          kaiju.state = "walk";
          kaiju.walkT = 0;
          kaiju.lastHit = kaiju.target;
          kaiju.target = -1;
        }
      }
    }

    // the beam collapses its target; everything else rebuilds
    for (var k = 0; k < buildings.length; k++) {
      var b = buildings[k];
      if (firing && k === kaiju.target) {
        b.type = "collapse";
        b.dmg = Math.min(1, b.dmg + dt / (FIRE_MS * 0.5));
      } else if (animate) {
        b.dmg = Math.max(0, b.dmg - dt / 1500);
        if (b.dmg <= 0) b.type = null;
      }
    }

    var frameIndex;
    if (kaiju.state === "fire") frameIndex = FRAMES.fire;
    else if (kaiju.state === "charge") frameIndex = FRAMES.charge;
    else if (kaiju.state === "walk") frameIndex = Math.floor(now / 170) % 2 ? FRAMES.walkB : FRAMES.walkA;
    else frameIndex = FRAMES.idle[Math.floor(now / 230) % FRAMES.idle.length]; // idle pulse

    // beam first (behind the kaiju) so it emerges from the mouth without
    // covering the head; kaiju over it; buildings in front of all
    if (firing && kaiju.target >= 0) drawBeam(mouthX, mouthY, buildings[kaiju.target], now, dh);
    drawKaiju(kaiju.x, baseY, frameIndex, kaiju.state === "walk", now, kaiju.dir);
    for (var j = 0; j < buildings.length; j++) drawBuilding(buildings[j], now);

    // ground-impact ripples from footfalls
    for (var s = stomps.length - 1; s >= 0; s--) {
      var el = now - stomps[s].t0;
      if (el > 1300) {
        stomps.splice(s, 1);
        continue;
      }
      var rad = (el / 1000) * 150;
      ctx.strokeStyle = rgba(EMBER, (1 - el / 1300) * 0.45);
      ctx.lineWidth = 1.4;
      ctx.beginPath();
      ctx.ellipse(stomps[s].x, h - 2, rad, rad * 0.16, 0, Math.PI, Math.PI * 2);
      ctx.stroke();
    }
  }

  var raf = 0;
  function loop(now) {
    frame(now, true);
    raf = window.requestAnimationFrame(loop);
  }

  function start() {
    // skip the hero animation entirely on mobile (canvas is hidden via CSS too)
    if (window.matchMedia && window.matchMedia("(max-width: 768px)").matches) return;
    build();
    var reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    sprite.onload = function () {
      spriteReady = true;
      if (reduce) {
        kaiju.x = w * 0.62; // a static standing pose
        frame(0, false);
      }
    };
    sprite.src = canvas.getAttribute("data-sprites") || "";

    if (!reduce) {
      kaiju.x = w * 0.55; // start in the visible right half
      raf = window.requestAnimationFrame(loop);
    }
  }

  var resizeTimer = 0;
  window.addEventListener("resize", function () {
    window.clearTimeout(resizeTimer);
    resizeTimer = window.setTimeout(build, 180);
  });

  start();
})();

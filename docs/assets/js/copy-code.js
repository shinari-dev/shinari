// SPDX-License-Identifier: Apache-2.0
// © The Shinari Authors

// Attach a copy-to-clipboard button to the top-right corner of every code block:
// markdown fences (.highlight) and the hand-written homepage panels alike.
(function () {
  "use strict";

  var COPY_SVG =
    '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>';
  var CHECK_SVG =
    '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>';

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      try {
        var ta = document.createElement("textarea");
        ta.value = text;
        ta.setAttribute("readonly", "");
        ta.style.position = "absolute";
        ta.style.left = "-9999px";
        document.body.appendChild(ta);
        ta.select();
        var ok = document.execCommand("copy");
        document.body.removeChild(ta);
        ok ? resolve() : reject(new Error("copy command rejected"));
      } catch (e) {
        reject(e);
      }
    });
  }

  function attach(code) {
    var pre = code.parentElement;
    if (!pre || pre.tagName !== "PRE") return;

    // Anchor the button to a non-scrolling host so it stays pinned to the
    // corner even when the <pre> itself scrolls horizontally.
    var host = pre.closest(".code-panel, .terminal, .highlight") || pre;
    if (host.classList.contains("has-copy")) return; // one block, one button
    host.classList.add("has-copy");

    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "copy-btn";
    btn.setAttribute("aria-label", "Copy code to clipboard");
    btn.title = "Copy";
    btn.innerHTML = COPY_SVG;

    var reset;
    btn.addEventListener("click", function () {
      var text = code.innerText.replace(/\n+$/, "");
      copyText(text).then(
        function () {
          btn.classList.remove("copy-fail");
          btn.classList.add("copied");
          btn.innerHTML = CHECK_SVG;
          btn.title = "Copied";
          window.clearTimeout(reset);
          reset = window.setTimeout(function () {
            btn.classList.remove("copied");
            btn.innerHTML = COPY_SVG;
            btn.title = "Copy";
          }, 1800);
        },
        function () {
          btn.classList.remove("copied");
          btn.classList.add("copy-fail");
          btn.title = "Copy failed";
          window.clearTimeout(reset);
          reset = window.setTimeout(function () {
            btn.classList.remove("copy-fail");
            btn.title = "Copy";
          }, 1800);
        }
      );
    });

    host.appendChild(btn);
  }

  function init() {
    document.querySelectorAll("pre > code").forEach(attach);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();

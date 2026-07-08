// SPDX-License-Identifier: Apache-2.0
// © The Shinari Authors

// Fetch the GitHub star count once and paint it into every [data-gh-stars]
// slot (the header button and the support banner). Also wires the dismissible
// "star us" banner, remembered per visitor in localStorage.
(function () {
  "use strict";

  var CACHE_KEY = "shinari:star-count";
  var CACHE_TTL = 6 * 60 * 60 * 1000; // 6h: stay well under the anonymous API rate limit

  function repoSlug(url) {
    // https://github.com/owner/name(.git)? -> "owner/name"
    var m = /github\.com[/:]([^/]+\/[^/]+?)(?:\.git)?\/?$/.exec(url || "");
    return m ? m[1] : null;
  }

  function format(n) {
    if (typeof Intl !== "undefined" && Intl.NumberFormat) {
      return new Intl.NumberFormat().format(n);
    }
    return String(n);
  }

  function paint(count) {
    if (typeof count !== "number" || count < 0) return;
    var text = format(count);
    document.querySelectorAll("[data-gh-stars]").forEach(function (el) {
      el.textContent = text;
      el.hidden = false;
    });
  }

  function readCache() {
    try {
      var raw = window.localStorage.getItem(CACHE_KEY);
      if (!raw) return null;
      var v = JSON.parse(raw);
      if (!v || typeof v.count !== "number" || !v.at) return null;
      return v;
    } catch (e) {
      return null;
    }
  }

  function writeCache(count, now) {
    try {
      window.localStorage.setItem(CACHE_KEY, JSON.stringify({ count: count, at: now }));
    } catch (e) {
      /* storage unavailable; skip caching */
    }
  }

  function loadStars(slug) {
    var now = Date.now();
    var cached = readCache();
    if (cached) {
      paint(cached.count);
      if (now - cached.at < CACHE_TTL) return; // fresh enough, no request
    }
    if (!window.fetch) return;
    fetch("https://api.github.com/repos/" + slug, {
      headers: { Accept: "application/vnd.github+json" },
    })
      .then(function (r) {
        return r.ok ? r.json() : null;
      })
      .then(function (data) {
        if (data && typeof data.stargazers_count === "number") {
          paint(data.stargazers_count);
          writeCache(data.stargazers_count, now);
        }
      })
      .catch(function () {
        /* offline or rate-limited; the cached or hidden state stands */
      });
  }

  function init() {
    var host = document.querySelector("[data-gh-repo]");
    var slug = host && repoSlug(host.getAttribute("data-gh-repo"));
    if (slug) loadStars(slug);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();

// Watchdog Analytics Beacon
(function () {
  "use strict";

  // Configuration from script tag data attributes
  var scriptEl = document.currentScript;
  var config = {
    endpoint: scriptEl.getAttribute("data-api") || "/api/event",
    domain: scriptEl.getAttribute("data-domain") || window.location.hostname,
    hashMode: scriptEl.hasAttribute("data-hash-mode"),
    outboundLinks: scriptEl.hasAttribute("data-outbound-links"),
    fileDownloads: scriptEl.hasAttribute("data-file-downloads"),
    exclude: scriptEl.getAttribute("data-exclude") || "",
    manual: scriptEl.hasAttribute("data-manual"),
  };

  var tracked = false;
  var lastPage = null;

  // Parse exclusions (comma-separated paths)
  var exclusions = config.exclude
    ? config.exclude.split(",").map(function (s) {
        return s.trim();
      })
    : [];

  // Check if page should be tracked
  function shouldTrack() {
    // Skip localhost unless explicitly allowed
    if (
      window.location.hostname === "localhost" ||
      window.location.hostname === "127.0.0.1"
    ) {
      return false;
    }

    // Check exclusions
    var path = window.location.pathname;
    for (var i = 0; i < exclusions.length; i++) {
      if (path.indexOf(exclusions[i]) === 0) {
        return false;
      }
    }

    return true;
  }

  // Send analytics payload to server
  function sendBeacon(payload) {
    if (!shouldTrack()) return;

    var data = JSON.stringify(payload);

    // Try navigator.sendBeacon first (best for page unload)
    if (navigator.sendBeacon) {
      var blob = new Blob([data], { type: "application/json" });
      navigator.sendBeacon(config.endpoint, blob);
      return;
    }

    // Fallback to fetch for browsers without sendBeacon
    if (window.fetch) {
      fetch(config.endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: data,
        keepalive: true,
      }).catch(function () {
        // Silently fail, analytics shouldn't break the page
      });
      return;
    }

    // Final fallback to XMLHttpRequest
    try {
      var xhr = new XMLHttpRequest();
      xhr.open("POST", config.endpoint, true);
      xhr.setRequestHeader("Content-Type", "application/json");
      xhr.send(data);
    } catch (e) {
      // Silently fail
    }
  }

  // Build payload
  function buildPayload(opts) {
    opts = opts || {};
    return {
      d: config.domain,
      p: opts.path || window.location.pathname + window.location.search,
      r: opts.referrer !== undefined ? opts.referrer : document.referrer || "",
      w: window.screen.width || 0,
    };
  }

  // Track a pageview
  function trackPageview(opts) {
    opts = opts || {};

    // Get current page (with hash if hash-mode is enabled)
    var currentPage = window.location.pathname + window.location.search;
    if (config.hashMode) {
      currentPage += window.location.hash;
    }

    // Avoid duplicate pageviews
    if (lastPage === currentPage && !opts.force) {
      return;
    }

    lastPage = currentPage;
    tracked = true;

    var payload = buildPayload({
      path: currentPage,
      referrer: opts.referrer,
    });

    sendBeacon(payload);
  }

  // Track a custom event
  function trackEvent(eventName, opts) {
    if (!eventName || typeof eventName !== "string") {
      console.warn("Watchdog: event name must be a non-empty string");
      return;
    }

    opts = opts || {};
    var payload = buildPayload(opts);
    payload.e = eventName;
    sendBeacon(payload);
  }

  // Track outbound link clicks
  function trackOutboundLink(event) {
    var link = event.target.closest("a");
    if (!link) return;

    var url = link.getAttribute("href");
    if (!url || url.indexOf("://") === -1) return;

    // Check if external
    var linkHostname = link.hostname;
    if (linkHostname === window.location.hostname) return;

    // Track as custom event
    trackEvent("Outbound Link: Click", {
      path: window.location.pathname,
      referrer: url,
    });
  }

  // Track file downloads
  function trackFileDownload(event) {
    var link = event.target.closest("a");
    if (!link) return;

    var href = link.getAttribute("href");
    if (!href) return;

    // Common file extensions.
    // FIXME: this needs to be more robust in the future
    var extensions = [
      ".pdf",
      ".zip",
      ".tar",
      ".gz",
      ".doc",
      ".docx",
      ".xls",
      ".xlsx",
      ".ppt",
      ".pptx",
    ];

    var isDownload = false;
    for (var i = 0; i < extensions.length; i++) {
      if (href.toLowerCase().indexOf(extensions[i]) !== -1) {
        isDownload = true;
        break;
      }
    }

    if (!isDownload) return;

    // Track as custom event
    trackEvent("File Download", {
      path: href,
    });
  }

  // Setup automatic tracking features
  function setupAutomaticTracking() {
    // Hash mode, track when hash changes (for SPAs)
    if (config.hashMode) {
      window.addEventListener("hashchange", function () {
        trackPageview();
      });
    }

    // Outbound link tracking
    if (config.outboundLinks) {
      document.addEventListener("click", trackOutboundLink);
    }

    // File download tracking
    if (config.fileDownloads) {
      document.addEventListener("click", trackFileDownload);
    }
  }

  // Expose public API
  window.watchdog = {
    track: trackEvent,
    trackPageview: trackPageview,
  };

  // Auto-track pageview on load (unless manual mode)
  if (!config.manual) {
    if (document.readyState === "complete") {
      trackPageview();
    } else {
      window.addEventListener("load", trackPageview);
    }
  }

  // Setup automatic tracking features
  setupAutomaticTracking();
})();

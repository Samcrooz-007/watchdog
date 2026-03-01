// Watchdog Analytics Beacon
(function () {
  "use strict";

  var endpoint = "/api/event";
  var tracked = false;

  // Send analytics payload to server
  function sendBeacon(payload) {
    var data = JSON.stringify(payload);

    // Try navigator.sendBeacon first (best for page unload)
    if (navigator.sendBeacon) {
      var blob = new Blob([data], { type: "application/json" });
      navigator.sendBeacon(endpoint, blob);
      return;
    }

    // Fallback to fetch for browsers without sendBeacon
    if (window.fetch) {
      fetch(endpoint, {
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
      xhr.open("POST", endpoint, true);
      xhr.setRequestHeader("Content-Type", "application/json");
      xhr.send(data);
    } catch (e) {
      // Silently fail
    }
  }

  // Build payload
  function buildPayload() {
    return {
      d: window.location.hostname,
      p: window.location.pathname,
      r: document.referrer || "",
      w: window.screen.width || 0,
    };
  }

  // Track a pageview
  function trackPageview() {
    if (tracked) return; // Only track once per page load
    tracked = true;
    sendBeacon(buildPayload());
  }

  // Track a custom event
  function trackEvent(eventName) {
    if (!eventName || typeof eventName !== "string") {
      console.warn("Watchdog: event name must be a non-empty string");
      return;
    }

    var payload = buildPayload();
    payload.e = eventName;
    sendBeacon(payload);
  }

  // Expose public API
  window.watchdog = {
    track: trackEvent,
    trackPageview: trackPageview,
  };

  // Auto-track pageview on load
  if (document.readyState === "complete") {
    trackPageview();
  } else {
    window.addEventListener("load", trackPageview);
  }
})();

(function () {
  var currentJobId = "";
  var currentResult = null;
  var requestToken = "";
  var basePath = "/";
  var benchmarkRunning = false;
  var keepAliveHandle = 0;
  var layoutDragCleanup = null;
  var sessionId = "";

  function byId(id) {
    return document.getElementById(id);
  }

  function rootStyle() {
    return document.documentElement.style;
  }

  function nowLabel() {
    return new Date().toLocaleTimeString();
  }

  function createSessionId() {
    if (window.crypto && window.crypto.randomUUID) {
      return window.crypto.randomUUID();
    }
    return "session-" + Date.now() + "-" + Math.random().toString(16).slice(2);
  }

  function loadRequestToken() {
    var tag = document.querySelector('meta[name="namebench-request-token"]');
    requestToken = tag ? tag.getAttribute("content") || "" : "";
    var basePathTag = document.querySelector('meta[name="namebench-base-path"]');
    basePath = normalizeBasePath(basePathTag ? basePathTag.getAttribute("content") || "/" : "/");
  }

  function authHeaders(extra) {
    var headers = Object.assign({}, extra || {});
    headers["X-Namebench-Token"] = requestToken;
    return headers;
  }

  function normalizeBasePath(path) {
    var value = String(path || "/").trim();
    if (!value) {
      return "/";
    }
    if (value.charAt(0) !== "/") {
      value = "/" + value;
    }
    if (value.charAt(value.length - 1) !== "/") {
      value += "/";
    }
    return value;
  }

  function apiPath(path) {
    var value = String(path || "");
    while (value.charAt(0) === "/") {
      value = value.slice(1);
    }
    return basePath + value;
  }

  async function openSession() {
    sessionId = createSessionId();
    try {
      await fetch(apiPath("session/open"), {
        method: "POST",
        credentials: "same-origin",
        headers: authHeaders({
          "Content-Type": "application/json; charset=UTF-8"
        }),
        body: JSON.stringify({ session_id: sessionId })
      });
    } catch (error) {
      // Ignore session-open failure. Server idle fallback still exists.
    }
  }

  function closeSession() {
    if (!sessionId) {
      return;
    }
    try {
      fetch(apiPath("session/close"), {
        method: "POST",
        keepalive: true,
        credentials: "same-origin",
        headers: authHeaders({
          "Content-Type": "application/json; charset=UTF-8"
        }),
        body: JSON.stringify({ session_id: sessionId })
      });
    } catch (error) {
      // Ignore close errors during page unload.
    }
  }

  function setStatus(state, text) {
    var dot = byId("status-dot");
    var statusText = byId("status-text");

    dot.classList.remove("is-running", "is-error");
    if (state === "running") {
      dot.classList.add("is-running");
    }
    if (state === "error") {
      dot.classList.add("is-error");
    }
    statusText.textContent = text;
  }

  function setProgress(percent, text, detail) {
    var normalized = Math.max(0, Math.min(100, asNumber(percent, 0)));
    byId("progress-fill").style.width = normalized + "%";
    byId("progress-percent").textContent = normalized.toFixed(0) + "%";
    byId("progress-text").textContent = text || "Idle";
    byId("progress-detail").textContent = detail || "0 / 0 queries";
  }

  function asNumber(value, fallback) {
    var n = Number(value);
    if (!Number.isFinite(n)) {
      return fallback;
    }
    return n;
  }

  function formatMs(value) {
    return asNumber(value, 0).toFixed(2);
  }

  function formatPct01(value) {
    return (asNumber(value, 0) * 100).toFixed(1) + "%";
  }

  function formatDuration(seconds) {
    var total = Math.max(0, Math.round(asNumber(seconds, 0)));
    var mins = Math.floor(total / 60);
    var secs = total % 60;
    if (mins > 0) {
      return mins + "m " + String(secs).padStart(2, "0") + "s";
    }
    return secs + "s";
  }

  function shortServer(server) {
    if (!server) {
      return "-";
    }
    return server.replace(/:53$/, "");
  }

  function updateNameserverCount() {
    var raw = byId("nameservers").value || "";
    var count = raw
      .split(/[\n,;\t ]+/)
      .map(function (part) {
        return part.trim();
      })
      .filter(Boolean).length;
    byId("nameserver-count").textContent = count + (count === 1 ? " entry" : " entries");
  }

  function setLog(text) {
    byId("result-log").textContent = text;
    byId("metric-updated").textContent = nowLabel();
  }

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function setSetupWidth(width) {
    var layout = byId("layout-grid");
    if (!layout) {
      return;
    }
    var bounds = layout.getBoundingClientRect();
    var dividerWidth = 18;
    var minWidth = 440;
    var maxWidth = Math.max(minWidth, Math.min(820, bounds.width - dividerWidth - 420));
    var nextWidth = clamp(width, minWidth, maxWidth);
    rootStyle().setProperty("--setup-width", nextWidth + "px");
    try {
      window.localStorage.setItem("namebench-setup-width", String(nextWidth));
    } catch (error) {
      // Ignore storage failures.
    }
  }

  function applySavedSetupWidth() {
    var stored = "";
    try {
      stored = window.localStorage.getItem("namebench-setup-width") || "";
    } catch (error) {
      stored = "";
    }
    if (!stored) {
      return;
    }
    var width = Number(stored);
    if (Number.isFinite(width) && width > 0) {
      setSetupWidth(width);
    }
  }

  function bindLayoutResizer() {
    var layout = byId("layout-grid");
    var divider = byId("layout-divider");
    if (!layout || !divider || window.matchMedia("(max-width: 1100px)").matches) {
      return;
    }

    divider.addEventListener("pointerdown", function (event) {
      event.preventDefault();
      divider.classList.add("is-dragging");
      var pointerId = event.pointerId;
      divider.setPointerCapture(pointerId);

      function move(clientX) {
        var bounds = layout.getBoundingClientRect();
        setSetupWidth(clientX - bounds.left);
      }

      function onPointerMove(moveEvent) {
        move(moveEvent.clientX);
      }

      function stop() {
        divider.classList.remove("is-dragging");
        divider.releasePointerCapture(pointerId);
        divider.removeEventListener("pointermove", onPointerMove);
        divider.removeEventListener("pointerup", stop);
        divider.removeEventListener("pointercancel", stop);
      }

      divider.addEventListener("pointermove", onPointerMove);
      divider.addEventListener("pointerup", stop);
      divider.addEventListener("pointercancel", stop);
    });

    divider.addEventListener("keydown", function (event) {
      var step = event.shiftKey ? 40 : 20;
      var current = Number(getComputedStyle(document.documentElement).getPropertyValue("--setup-width").replace("px", "")) || 620;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        setSetupWidth(current - step);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        setSetupWidth(current + step);
      }
    });

    function onResize() {
      var current = Number(getComputedStyle(document.documentElement).getPropertyValue("--setup-width").replace("px", "")) || 620;
      setSetupWidth(current);
    }

    window.addEventListener("resize", onResize);
    layoutDragCleanup = function () {
      window.removeEventListener("resize", onResize);
    };
  }

  function setLogExpanded(expanded) {
    var panel = byId("result-log-panel");
    var toggle = byId("log-toggle");
    var label = byId("log-toggle-label");
    panel.hidden = !expanded;
    toggle.setAttribute("aria-expanded", expanded ? "true" : "false");
    label.textContent = expanded ? "Hide" : "Show";
  }

  function toggleLogPanel() {
    var expanded = byId("log-toggle").getAttribute("aria-expanded") === "true";
    setLogExpanded(!expanded);
  }

  function parseJsonSafe(text) {
    try {
      return JSON.parse(text);
    } catch (error) {
      return null;
    }
  }

  function updateMetrics(data) {
    var results = Array.isArray(data.results) ? data.results : [];
    var totalFailures = results.reduce(function (acc, current) {
      return acc + asNumber(current.failures, 0);
    }, 0);

    byId("metric-server").textContent = shortServer(data.winner || "-");
    byId("metric-queries").textContent = String(asNumber(data.executed_queries, 0));
    byId("metric-failures").textContent = String(totalFailures);
  }

  function bestResult(results, selector) {
    if (!results || results.length === 0) {
      return null;
    }
    return results.slice().sort(function (left, right) {
      var delta = selector(left) - selector(right);
      if (delta !== 0) {
        return delta;
      }
      return (left.server || "").localeCompare(right.server || "");
    })[0];
  }

  function setSummaryCard(valueId, detailId, value, detail) {
    byId(valueId).textContent = value || "-";
    byId(detailId).textContent = detail || "-";
  }

  function resetSummaryCards() {
    setSummaryCard("summary-winner", "summary-winner-detail", "-", "Best overall score");
    setSummaryCard("summary-cold", "summary-cold-detail", "-", "Lowest uncached average");
    setSummaryCard("summary-warm", "summary-warm-detail", "-", "Lowest cached average");
    setSummaryCard("summary-stable", "summary-stable-detail", "-", "Lowest failure rate and jitter");
  }

  function updateSummaryCards(data) {
    var results = Array.isArray(data.results) ? data.results : [];
    if (results.length === 0) {
      resetSummaryCards();
      return;
    }

    var winnerResult = results.find(function (result) {
      return result.server === data.winner;
    }) || results[0];
    var coldResult = bestResult(results, function (result) {
      return asNumber(result.uncached_avg_ms, 0);
    });
    var warmResult = bestResult(results, function (result) {
      return asNumber(result.cached_avg_ms, 0);
    });
    var stableResult = results.slice().sort(function (left, right) {
      var failDelta = asNumber(left.failure_rate, 0) - asNumber(right.failure_rate, 0);
      if (failDelta !== 0) {
        return failDelta;
      }
      var jitterDelta = asNumber(left.jitter_ms, 0) - asNumber(right.jitter_ms, 0);
      if (jitterDelta !== 0) {
        return jitterDelta;
      }
      return (left.server || "").localeCompare(right.server || "");
    })[0];

    setSummaryCard(
      "summary-winner",
      "summary-winner-detail",
      shortServer(winnerResult.server),
      "score " + formatMs(winnerResult.score) + " • integrity " + (winnerResult.integrity || "unknown")
    );
    setSummaryCard(
      "summary-cold",
      "summary-cold-detail",
      shortServer(coldResult.server),
      formatMs(coldResult.uncached_avg_ms) + " ms uncached"
    );
    setSummaryCard(
      "summary-warm",
      "summary-warm-detail",
      shortServer(warmResult.server),
      formatMs(warmResult.cached_avg_ms) + " ms cached"
    );
    setSummaryCard(
      "summary-stable",
      "summary-stable-detail",
      shortServer(stableResult.server),
      formatPct01(stableResult.failure_rate) + " fail • " + formatMs(stableResult.jitter_ms) + " ms jitter"
    );
  }

  function updateWinnerBanner(data) {
    var winner = shortServer(data.winner || "");
    var winnerBanner = byId("winner-banner");
    if (!winner || winner === "-") {
      winnerBanner.textContent = "No winner yet. Check warnings in the log.";
      return;
    }
    var results = Array.isArray(data.results) ? data.results : [];
    var winnerResult = results.find(function (result) {
      return result.server === data.winner;
    });
    var winnerTag = winnerResult ? formatResolverTag(winnerResult) : "";
    winnerBanner.textContent =
      "Winner: " +
      winner +
      (winnerTag ? " • " + winnerTag : "") +
      " (tested " +
      asNumber(data.server_count, 0) +
      " servers)";
  }

  function formatResolverTag(result) {
    var pieces = [];
    if (result && result.resolver_asn) {
      pieces.push("AS" + result.resolver_asn);
    }
    if (result && result.resolver_country) {
      pieces.push(result.resolver_country);
    }
    if (pieces.length === 0 && result && result.resolver_organization) {
      pieces.push(result.resolver_organization);
    }
    return pieces.join(" • ");
  }

  function setDiagnosticsSummary(summary) {
    byId("diag-clean").textContent = String(summary.clean);
    byId("diag-suspicious").textContent = String(summary.suspicious);
    byId("diag-hijacked").textContent = String(summary.hijacked);
    byId("diag-unknown").textContent = String(summary.unknown);

    var banner = byId("diagnostics-banner");
    if (summary.hijacked > 0) {
      banner.textContent = summary.hijacked + " resolver returned redirected answers for NXDOMAIN probes.";
      return;
    }
    if (summary.suspicious > 0) {
      banner.textContent = summary.suspicious + " resolver needs manual review.";
      return;
    }
    if (summary.clean > 0) {
      banner.textContent = "All reported resolvers handled NXDOMAIN probes cleanly.";
      return;
    }
    banner.textContent = "Run benchmark to inspect resolver integrity.";
  }

  function setExportButtons(enabled) {
    byId("export-json-button").disabled = !enabled;
    byId("export-csv-button").disabled = !enabled;
  }

  function hasHistoryConsent() {
    return byId("history-consent").checked;
  }

  function refreshBenchmarkEligibility() {
    byId("start-button").disabled = benchmarkRunning || !hasHistoryConsent();
  }

  function clearNode(node) {
    while (node.firstChild) {
      node.removeChild(node.firstChild);
    }
  }

  function createCell(row, text, className, title) {
    var cell = document.createElement("td");
    cell.textContent = text;
    if (className) {
      cell.className = className;
    }
    if (title) {
      cell.title = title;
    }
    row.appendChild(cell);
  }

  function renderResultsTable(data) {
    var tbody = byId("results-body");
    clearNode(tbody);

    var results = Array.isArray(data.results) ? data.results : [];
    if (results.length === 0) {
      var emptyRow = document.createElement("tr");
      var emptyCell = document.createElement("td");
      emptyCell.colSpan = 11;
      emptyCell.className = "empty-cell";
      emptyCell.textContent = "No benchmark data returned.";
      emptyRow.appendChild(emptyCell);
      tbody.appendChild(emptyRow);
      return;
    }

    results.forEach(function (result) {
      var row = document.createElement("tr");
      if (result.server === data.winner) {
        row.classList.add("winner-row");
      }

      createCell(row, String(asNumber(result.rank, 0)));
      createCell(row, result.server || "-");
      createCell(
        row,
        formatResolverTag(result) || "-",
        "",
        [result.resolver_organization || "", result.resolver_as_name || "", result.resolver_ip || ""].filter(Boolean).join(" | ")
      );
      createCell(row, formatMs(result.score));
      createCell(row, formatMs(result.uncached_avg_ms));
      createCell(row, formatMs(result.cached_avg_ms));
      createCell(row, formatMs(result.jitter_ms));
      createCell(
        row,
        result.integrity || "-",
        result.integrity ? "integrity-" + result.integrity : "",
        result.integrity_detail || ""
      );
      createCell(row, formatPct01(result.failure_rate));
      createCell(row, String(asNumber(result.successes, 0)));
      createCell(row, String(asNumber(result.failures, 0)));

      tbody.appendChild(row);
    });
  }

  function renderBarChart(containerId, entries, valueFormatter, fillClassName) {
    var container = byId(containerId);
    clearNode(container);

    if (!entries || entries.length === 0) {
      container.classList.add("empty-state");
      container.textContent = "Not enough data to render chart.";
      return;
    }

    container.classList.remove("empty-state");
    var maxValue = entries.reduce(function (max, entry) {
      return Math.max(max, asNumber(entry.value, 0));
    }, 0);
    if (maxValue <= 0) {
      maxValue = 1;
    }

    entries.forEach(function (entry) {
      var row = document.createElement("div");
      row.className = "bar-row";

      var label = document.createElement("span");
      label.className = "bar-label";
      label.title = entry.label;
      label.textContent = entry.label;

      var track = document.createElement("span");
      track.className = "bar-track";

      var fill = document.createElement("span");
      fill.className = "bar-fill";
      if (fillClassName) {
        fill.classList.add(fillClassName);
      }
      track.appendChild(fill);

      var value = document.createElement("span");
      value.className = "bar-value";
      value.textContent = valueFormatter(entry.value);

      row.appendChild(label);
      row.appendChild(track);
      row.appendChild(value);
      container.appendChild(row);

      var rawWidth = Math.round((asNumber(entry.value, 0) / maxValue) * 100);
      var width = rawWidth <= 0 ? 0 : Math.max(2, rawWidth);
      requestAnimationFrame(function () {
        fill.style.width = width + "%";
      });
    });
  }

  function renderCharts(data) {
    var results = Array.isArray(data.results) ? data.results : [];

    var coldEntries = results.map(function (result) {
      return {
        label: shortServer(result.server),
        value: asNumber(result.uncached_avg_ms, 0)
      };
    });
    renderBarChart("cold-latency-bars", coldEntries, function (value) {
      return formatMs(value) + " ms";
    }, "");

    var warmEntries = results.map(function (result) {
      return {
        label: shortServer(result.server),
        value: asNumber(result.cached_avg_ms, 0)
      };
    });
    renderBarChart("warm-latency-bars", warmEntries, function (value) {
      return formatMs(value) + " ms";
    }, "dist-fill");

    var errorEntries = results.map(function (result) {
      return {
        label: shortServer(result.server),
        value: asNumber(result.failure_rate, 0) * 100
      };
    });
    renderBarChart("error-bars", errorEntries, function (value) {
      return value.toFixed(1) + "%";
    }, "error-fill");
  }

  function renderDiagnostics(data) {
    var results = Array.isArray(data.results) ? data.results : [];
    var summary = {
      clean: 0,
      suspicious: 0,
      hijacked: 0,
      unknown: 0
    };
    var list = byId("diagnostics-list");
    clearNode(list);

    results.forEach(function (result) {
      var status = result.integrity || "unknown";
      if (Object.prototype.hasOwnProperty.call(summary, status)) {
        summary[status] += 1;
      } else {
        summary.unknown += 1;
      }
    });
    setDiagnosticsSummary(summary);

    var flagged = results.filter(function (result) {
      return result.integrity && result.integrity !== "clean";
    });

    if (flagged.length === 0) {
      list.classList.add("empty-state");
      list.textContent = results.length === 0 ? "Run benchmark to render diagnostics." : "No suspicious NXDOMAIN or redirection behavior detected.";
      return;
    }

    list.classList.remove("empty-state");
    flagged.forEach(function (result) {
      var row = document.createElement("article");
      row.className = "diagnostic-row is-" + (result.integrity || "unknown");

      var head = document.createElement("div");
      head.className = "diagnostic-head";

      var server = document.createElement("span");
      server.className = "diagnostic-server";
      server.textContent = result.server || "-";

      var status = document.createElement("span");
      status.className = "diagnostic-status integrity-" + (result.integrity || "unknown");
      status.textContent = result.integrity || "unknown";

      var meta = document.createElement("p");
      meta.className = "diagnostic-meta";
      meta.textContent =
        (formatResolverTag(result) || "metadata unavailable") +
        " • " +
        "probes " +
        asNumber(result.integrity_probe_count, 0) +
        " • clean " +
        asNumber(result.integrity_clean_count, 0) +
        " • anomalies " +
        asNumber(result.integrity_anomaly_count, 0) +
        " • errors " +
        asNumber(result.integrity_error_count, 0);

      var detail = document.createElement("p");
      detail.className = "diagnostic-detail";
      detail.textContent = result.integrity_detail || "No additional detail.";

      head.appendChild(server);
      head.appendChild(status);
      row.appendChild(head);
      row.appendChild(meta);
      row.appendChild(detail);
      list.appendChild(row);
    });
  }

  function buildLogText(data) {
    var lines = [];
    lines.push("winner=" + (data.winner || "none"));
    lines.push("executed_queries=" + asNumber(data.executed_queries, 0));
    lines.push("servers_tested=" + asNumber(data.server_count, 0));

    var results = Array.isArray(data.results) ? data.results : [];
    results.forEach(function (result) {
      lines.push(
        [
          "#" + asNumber(result.rank, 0),
          result.server,
          "score=" + formatMs(result.score),
          "cold_avg_ms=" + formatMs(result.uncached_avg_ms),
          "warm_avg_ms=" + formatMs(result.cached_avg_ms),
          "jitter_ms=" + formatMs(result.jitter_ms),
          "integrity=" + (result.integrity || "unknown"),
          "fail_rate=" + formatPct01(result.failure_rate)
        ].join(" ")
      );
    });

    if (Array.isArray(data.warnings) && data.warnings.length > 0) {
      lines.push("warnings:");
      data.warnings.forEach(function (warning) {
        lines.push("- " + warning);
      });
    }

    return lines.join("\n");
  }

  function exportTimestamp() {
    return new Date().toISOString().replace(/[:.]/g, "-");
  }

  function downloadBlob(filename, content, mimeType) {
    var blob = new Blob([content], { type: mimeType });
    var url = window.URL.createObjectURL(blob);
    var link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    window.URL.revokeObjectURL(url);
  }

  function buildExportEnvelope(data) {
    return {
      exported_at: new Date().toISOString(),
      source: "namebench",
      result: data
    };
  }

  function escapeCsv(value) {
    var text = value == null ? "" : String(value);
    if (/[",\n]/.test(text)) {
      return "\"" + text.replace(/"/g, "\"\"") + "\"";
    }
    return text;
  }

  function buildCsv(data) {
    var rows = [
      [
        "rank",
        "server",
        "resolver_ip",
        "resolver_asn",
        "resolver_as_name",
        "resolver_country",
        "resolver_organization",
        "score",
        "uncached_avg_ms",
        "cached_avg_ms",
        "jitter_ms",
        "integrity",
        "integrity_detail",
        "failure_rate",
        "successes",
        "failures",
        "queries"
      ]
    ];

    var results = Array.isArray(data.results) ? data.results : [];
    results.forEach(function (result) {
      rows.push([
        asNumber(result.rank, 0),
        result.server || "",
        result.resolver_ip || "",
        result.resolver_asn || "",
        result.resolver_as_name || "",
        result.resolver_country || "",
        result.resolver_organization || "",
        formatMs(result.score),
        formatMs(result.uncached_avg_ms),
        formatMs(result.cached_avg_ms),
        formatMs(result.jitter_ms),
        result.integrity || "",
        result.integrity_detail || "",
        asNumber(result.failure_rate, 0),
        asNumber(result.successes, 0),
        asNumber(result.failures, 0),
        asNumber(result.queries, 0)
      ]);
    });

    return rows.map(function (row) {
      return row.map(escapeCsv).join(",");
    }).join("\n");
  }

  function exportJson() {
    if (!currentResult) {
      return;
    }
    downloadBlob(
      "namebench-" + exportTimestamp() + ".json",
      JSON.stringify(buildExportEnvelope(currentResult), null, 2),
      "application/json;charset=utf-8"
    );
  }

  function exportCsv() {
    if (!currentResult) {
      return;
    }
    downloadBlob(
      "namebench-" + exportTimestamp() + ".csv",
      buildCsv(currentResult),
      "text/csv;charset=utf-8"
    );
  }

  function buildProgressText(state) {
    var progress = state.progress || {};
    var lines = [];
    lines.push("job_id=" + (state.job_id || "unknown"));
    lines.push("status=" + (state.status || "unknown"));
    lines.push("progress=" + asNumber(progress.completed_steps, 0) + "/" + asNumber(progress.total_steps, 0));
    if (progress.current_server) {
      lines.push("server=" + progress.current_server);
    }
    if (progress.current_phase) {
      lines.push("phase=" + progress.current_phase);
    }
    if (asNumber(progress.elapsed_seconds, 0) > 0) {
      lines.push("elapsed=" + formatDuration(progress.elapsed_seconds));
    }
    if (asNumber(progress.eta_seconds, 0) > 0) {
      lines.push("eta=" + formatDuration(progress.eta_seconds));
    }
    return lines.join("\n");
  }

  function buildProgressDetail(progress) {
    var detail = [];
    detail.push(asNumber(progress.completed_steps, 0) + " / " + asNumber(progress.total_steps, 0) + " queries");
    if (asNumber(progress.elapsed_seconds, 0) > 0) {
      detail.push("elapsed " + formatDuration(progress.elapsed_seconds));
    }
    if (asNumber(progress.eta_seconds, 0) > 0) {
      detail.push("eta " + formatDuration(progress.eta_seconds));
    }
    return detail.join(" • ");
  }

  function getConfigPayload() {
    return {
      nameservers: byId("nameservers").value || "",
      include_system: byId("include-system").checked,
      include_metadata: byId("include-metadata").checked,
      include_global: byId("include-global").checked,
      include_regional: byId("include-regional").checked,
      history_consent: byId("history-consent").checked,
      location: byId("location").value || "",
      data_source: byId("data-source").value || "",
      query_count: asNumber(byId("query-count").value, 0)
    };
  }

  function setBenchmarkButtons(isRunning) {
    benchmarkRunning = isRunning;
    refreshBenchmarkEligibility();
    byId("cancel-button").disabled = !isRunning;
    byId("dnssec-button").disabled = isRunning;
  }

  async function pingServer() {
    try {
      var url = apiPath("ping") + (sessionId ? ("?session_id=" + encodeURIComponent(sessionId)) : "");
      await fetch(url, {
        credentials: "same-origin",
        headers: authHeaders({
          "Accept": "application/json"
        })
      });
    } catch (error) {
      // Ignore keepalive failures. They usually mean the app is already shutting down.
    }
  }

  async function pollBenchmark(jobId) {
    currentJobId = jobId;

    while (currentJobId === jobId) {
      var response = await fetch(apiPath("progress") + "?job_id=" + encodeURIComponent(jobId), {
        credentials: "same-origin",
        headers: {
          "Accept": "application/json",
          "X-Namebench-Token": requestToken
        }
      });
      var raw = await response.text();
      var state = parseJsonSafe(raw);
      if (!response.ok || !state) {
        throw new Error(raw || ("HTTP " + response.status));
      }

      var progress = state.progress || {};
      var progressLabel = "Idle";
      if (state.status === "running") {
        progressLabel = (progress.current_phase || "benchmarking") + (progress.current_server ? " " + progress.current_server : "");
      } else if (state.status === "completed") {
        progressLabel = "Completed";
      } else if (state.status === "canceled") {
        progressLabel = "Canceled";
      } else if (state.status === "error") {
        progressLabel = "Error";
      }
      setProgress(progress.percent || 0, progressLabel, buildProgressDetail(progress));

      if (state.status === "completed" && state.result) {
        currentResult = state.result;
        setExportButtons(true);
        updateMetrics(state.result);
        updateSummaryCards(state.result);
        updateWinnerBanner(state.result);
        renderResultsTable(state.result);
        renderDiagnostics(state.result);
        renderCharts(state.result);
        setLog(buildLogText(state.result));
        setLogExpanded(false);
        setStatus("ready", "Benchmark completed");
        currentJobId = "";
        return;
      }

      if (state.status === "canceled") {
        setStatus("error", "Benchmark canceled");
        resetSummaryCards();
        setLog(buildProgressText(state));
        setLogExpanded(true);
        currentJobId = "";
        return;
      }

      if (state.status === "error") {
        setStatus("error", "Benchmark failed");
        resetSummaryCards();
        setLog("error: " + (state.error || "unknown"));
        setLogExpanded(true);
        currentJobId = "";
        return;
      }

      setStatus("running", "Benchmark running");
      setLog(buildProgressText(state));
      await new Promise(function (resolve) {
        window.setTimeout(resolve, 700);
      });
    }
  }

  async function runBenchmark(event) {
    event.preventDefault();
    var form = byId("benchmark-form");
    currentResult = null;
    setExportButtons(false);
    resetSummaryCards();
    setBenchmarkButtons(true);
    setProgress(0, "Starting benchmark", "0 / 0 queries");
    setStatus("running", "Benchmark running");
    setDiagnosticsSummary({ clean: 0, suspicious: 0, hijacked: 0, unknown: 0 });
    byId("diagnostics-list").classList.add("empty-state");
    byId("diagnostics-list").textContent = "Benchmark running. Diagnostics will appear when integrity probes finish.";
    setLog("benchmark started...");
    setLogExpanded(false);

    try {
      var response = await fetch(form.action, {
        method: "POST",
        credentials: "same-origin",
        headers: authHeaders({
          "Accept": "application/json",
          "Content-Type": "application/json; charset=UTF-8"
        }),
        body: JSON.stringify(getConfigPayload())
      });

      var raw = await response.text();
      var data = parseJsonSafe(raw);

      if (!response.ok) {
        throw new Error(raw || ("HTTP " + response.status));
      }

      if (!data) {
        throw new Error("Benchmark endpoint did not return JSON.");
      }
      if (!data.job_id) {
        throw new Error("Benchmark endpoint did not return a job id.");
      }
      await pollBenchmark(data.job_id);
    } catch (error) {
      currentResult = null;
      setStatus("error", "Benchmark failed");
      resetSummaryCards();
      setLog("error: " + error.message);
      setLogExpanded(true);
    } finally {
      setBenchmarkButtons(false);
      if (!currentJobId) {
        setProgress(0, "Idle", "0 / 0 queries");
      }
    }
  }

  async function cancelBenchmark() {
    if (!currentJobId) {
      return;
    }

    try {
      await fetch(apiPath("cancel"), {
        method: "POST",
        credentials: "same-origin",
        headers: authHeaders({
          "Content-Type": "application/json; charset=UTF-8"
        }),
        body: JSON.stringify({ job_id: currentJobId })
      });
    } finally {
      currentJobId = "";
      currentResult = null;
      setExportButtons(false);
      resetSummaryCards();
      setBenchmarkButtons(false);
      setStatus("error", "Benchmark canceled");
      setProgress(0, "Canceled", "0 / 0 queries");
      setLog("benchmark canceled.");
      setLogExpanded(true);
    }
  }

  async function runDnssecCheck() {
    var button = byId("dnssec-button");
    button.disabled = true;
    setStatus("running", "DNSSEC checks in progress");
    setLog("dnssec check started...");

    try {
      var response = await fetch(apiPath("dnssec"), {
        method: "POST",
        credentials: "same-origin",
        headers: authHeaders({
          "Content-Type": "application/json; charset=UTF-8"
        }),
        body: JSON.stringify(getConfigPayload())
      });
      var raw = await response.text();
      if (!response.ok) {
        throw new Error(raw || ("HTTP " + response.status));
      }
      setLog(raw);
      setLogExpanded(true);
      setStatus("ready", "DNSSEC checks completed");
    } catch (error) {
      setStatus("error", "DNSSEC check failed");
      setLog("error: " + error.message);
      setLogExpanded(true);
    } finally {
      button.disabled = false;
    }
  }

  async function shutdownApp() {
    if (!window.confirm("Shut down the local namebench app now?")) {
      return;
    }

    byId("shutdown-button").disabled = true;
    currentJobId = "";
    currentResult = null;
    setExportButtons(false);
    setBenchmarkButtons(false);
    setStatus("running", "Shutting down");
    setProgress(0, "Shutting down", "Server is stopping");
    setLog("shutdown requested...");
    if (keepAliveHandle) {
      window.clearInterval(keepAliveHandle);
      keepAliveHandle = 0;
    }

    try {
      var response = await fetch(apiPath("shutdown"), {
        method: "POST",
        credentials: "same-origin",
        headers: authHeaders({
          "Accept": "application/json"
        })
      });
      var raw = await response.text();
      if (!response.ok) {
        throw new Error(raw || ("HTTP " + response.status));
      }
      setStatus("ready", "App shut down requested");
      setLog("shutdown requested. This page will stop responding when the local server exits.");
      window.setTimeout(function () {
        window.close();
      }, 800);
    } catch (error) {
      byId("shutdown-button").disabled = false;
      setStatus("error", "Shutdown failed");
      setLog("error: " + error.message);
      setLogExpanded(true);
      refreshBenchmarkEligibility();
      keepAliveHandle = window.setInterval(pingServer, 30000);
    }
  }

  loadRequestToken();
  byId("benchmark-form").addEventListener("submit", runBenchmark);
  byId("cancel-button").addEventListener("click", cancelBenchmark);
  byId("dnssec-button").addEventListener("click", runDnssecCheck);
  byId("shutdown-button").addEventListener("click", shutdownApp);
  byId("log-toggle").addEventListener("click", toggleLogPanel);
  byId("export-json-button").addEventListener("click", exportJson);
  byId("export-csv-button").addEventListener("click", exportCsv);
  byId("history-consent").addEventListener("change", refreshBenchmarkEligibility);
  byId("nameservers").addEventListener("input", updateNameserverCount);
  window.addEventListener("pagehide", closeSession);
  window.addEventListener("beforeunload", closeSession);
  document.addEventListener("visibilitychange", function () {
    if (!document.hidden) {
      pingServer();
    }
  });
  keepAliveHandle = window.setInterval(pingServer, 30000);
  openSession().finally(function () {
    pingServer();
  });
  applySavedSetupWidth();
  bindLayoutResizer();
  setDiagnosticsSummary({ clean: 0, suspicious: 0, hijacked: 0, unknown: 0 });
  setExportButtons(false);
  setLogExpanded(false);
  resetSummaryCards();
  updateNameserverCount();
  refreshBenchmarkEligibility();
  setProgress(0, "Idle", "0 / 0 queries");
})();

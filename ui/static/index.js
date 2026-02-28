(function () {
  function byId(id) {
    return document.getElementById(id);
  }

  function nowLabel() {
    return new Date().toLocaleTimeString();
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

  function shortServer(server) {
    if (!server) {
      return "-";
    }
    return server.replace(/:53$/, "");
  }

  function setLog(text) {
    byId("result-log").textContent = text;
    byId("metric-updated").textContent = nowLabel();
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

  function updateWinnerBanner(data) {
    var winner = shortServer(data.winner || "");
    var winnerBanner = byId("winner-banner");
    if (!winner || winner === "-") {
      winnerBanner.textContent = "No winner yet. Check warnings in the log.";
      return;
    }
    winnerBanner.textContent = "Winner: " + winner + " (tested " + asNumber(data.server_count, 0) + " servers)";
  }

  function clearNode(node) {
    while (node.firstChild) {
      node.removeChild(node.firstChild);
    }
  }

  function createCell(row, text) {
    var cell = document.createElement("td");
    cell.textContent = text;
    row.appendChild(cell);
  }

  function renderResultsTable(data) {
    var tbody = byId("results-body");
    clearNode(tbody);

    var results = Array.isArray(data.results) ? data.results : [];
    if (results.length === 0) {
      var emptyRow = document.createElement("tr");
      var emptyCell = document.createElement("td");
      emptyCell.colSpan = 8;
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
      createCell(row, formatMs(result.score));
      createCell(row, formatMs(result.avg_ms));
      createCell(row, formatMs(result.p95_ms));
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

    var latencyEntries = results.map(function (result) {
      return {
        label: shortServer(result.server),
        value: asNumber(result.avg_ms, 0)
      };
    });
    renderBarChart("latency-bars", latencyEntries, function (value) {
      return formatMs(value) + " ms";
    }, "");

    var errorEntries = results.map(function (result) {
      return {
        label: shortServer(result.server),
        value: asNumber(result.failure_rate, 0) * 100
      };
    });
    renderBarChart("error-bars", errorEntries, function (value) {
      return value.toFixed(1) + "%";
    }, "error-fill");

    var winner = results.find(function (result) {
      return result.server === data.winner;
    }) || results[0];

    var distributionEntries = [];
    if (winner && Array.isArray(winner.latency_buckets)) {
      distributionEntries = winner.latency_buckets.map(function (bucket) {
        return {
          label: bucket.label,
          value: asNumber(bucket.count, 0)
        };
      });
    }
    renderBarChart("distribution-bars", distributionEntries, function (value) {
      return String(asNumber(value, 0));
    }, "dist-fill");
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
          "avg_ms=" + formatMs(result.avg_ms),
          "p95_ms=" + formatMs(result.p95_ms),
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

  async function runBenchmark(event) {
    event.preventDefault();
    var form = byId("benchmark-form");
    var submitButton = byId("start-button");

    submitButton.disabled = true;
    setStatus("running", "Benchmark running");
    setLog("benchmark started...");

    try {
      var response = await fetch(form.action, {
        method: "POST",
        headers: { "Accept": "application/json" },
        body: new FormData(form)
      });

      var raw = await response.text();
      var data = parseJsonSafe(raw);

      if (!response.ok) {
        throw new Error(raw || ("HTTP " + response.status));
      }

      if (!data) {
        throw new Error("Benchmark endpoint did not return JSON.");
      }

      updateMetrics(data);
      updateWinnerBanner(data);
      renderResultsTable(data);
      renderCharts(data);
      setLog(buildLogText(data));
      setStatus("ready", "Benchmark completed");
    } catch (error) {
      setStatus("error", "Benchmark failed");
      setLog("error: " + error.message);
    } finally {
      submitButton.disabled = false;
    }
  }

  async function runDnssecCheck() {
    var button = byId("dnssec-button");
    var form = byId("benchmark-form");
    button.disabled = true;
    setStatus("running", "DNSSEC checks in progress");
    setLog("dnssec check started...");

    try {
      var response = await fetch("/dnssec", {
        method: "POST",
        body: new FormData(form)
      });
      var raw = await response.text();
      if (!response.ok) {
        throw new Error(raw || ("HTTP " + response.status));
      }
      setLog(raw);
      setStatus("ready", "DNSSEC checks completed");
    } catch (error) {
      setStatus("error", "DNSSEC check failed");
      setLog("error: " + error.message);
    } finally {
      button.disabled = false;
    }
  }

  byId("benchmark-form").addEventListener("submit", runBenchmark);
  byId("dnssec-button").addEventListener("click", runDnssecCheck);
})();

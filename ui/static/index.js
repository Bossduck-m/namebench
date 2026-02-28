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

  function parseBenchmark(raw) {
    var result = {
      server: "-",
      benchmarked: "0",
      failures: "0"
    };
    var matched = raw.match(/server=([^\s]+)\s+benchmarked=(\d+)\s+failures=(\d+)/);
    if (matched) {
      result.server = matched[1];
      result.benchmarked = matched[2];
      result.failures = matched[3];
    }
    return result;
  }

  function setLog(text) {
    byId("result-log").textContent = text;
    byId("metric-updated").textContent = nowLabel();
  }

  function syncMetrics(metrics) {
    byId("metric-server").textContent = metrics.server;
    byId("metric-queries").textContent = metrics.benchmarked;
    byId("metric-failures").textContent = metrics.failures;
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
        body: new FormData(form)
      });
      var raw = await response.text();
      if (!response.ok) {
        throw new Error(raw || ("HTTP " + response.status));
      }

      var metrics = parseBenchmark(raw);
      syncMetrics(metrics);
      setLog(raw);
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
    button.disabled = true;
    setStatus("running", "DNSSEC checks in progress");
    setLog("dnssec check started...");

    try {
      var response = await fetch("/dnssec", { method: "GET" });
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

# namebench (modernized fork)

namebench benchmarks DNS resolvers against sampled real-world hostnames from browser history, then ranks servers by latency and reliability.

## Current status

This fork adds:

- Modern responsive web UI
- Multi-server benchmark ranking
- Two-pass cache-aware benchmark (cold + warm resolver latency)
- Jitter/consistency scoring and NXDOMAIN integrity checks
- System DNS resolver discovery
- Resolver ASN / country labeling (opt-in)
- CSV / JSON result export
- Score table (rank, cold/warm avg, jitter, integrity, fail rate, success/fail)
- Visual charts (cold latency, warm latency, error rate)
- NXDOMAIN / redirection diagnostics panel
- DNSSEC quick-check endpoint

## Requirements

- Go 1.22+

## Build

```bash
git clone https://github.com/<your-user>/namebench.git
cd namebench
go mod tidy
go build ./...
```

## Run (recommended)

Run in HTTP mode:

```bash
# Linux/macOS
./namebench -port 8100

# Windows
.\\namebench.exe -port 8100
```

By default it auto-opens your browser. If browser auto-open is blocked, open manually:

- `http://127.0.0.1:8100/`

You can disable auto-open:

```bash
./namebench -port 8100 -open_browser=false
```

To enable verbose console logs:

```bash
./namebench -debug=true
```

By default logs are written to `namebench.log`.

## Windows: no terminal typing

You have two options:

1. One-time local build with double-click:
   - Double-click `build_windows_gui.bat`
   - It builds `namebench.exe` and launches it automatically

2. Prebuilt EXE from GitHub Actions artifact:
   - Go to `Actions` tab
   - Run workflow `Build Windows EXE` (or use a tagged run)
   - Download artifact `namebench-windows-amd64`
   - Extract and double-click `namebench.exe`

Default port is `8100`. You can override with `-port <value>`.

## How to use

1. Enter one or more DNS servers in the `Nameservers` box (one per line).
2. Optionally enable global/regional provider pools.
3. Optionally include the DNS servers already configured on this machine.
4. Set `Number of queries`.
5. Click `Start Benchmark`.
6. Review:
   - Winner banner
   - Ranked results table with ASN/country labels, cold/warm cache averages, jitter, and integrity
   - Export buttons for JSON and CSV result files
   - NXDOMAIN / redirection report with clean, suspicious, hijacked, and unknown resolver counts
   - Cold cache, warm cache, and error-rate charts
   - Live progress bar while the benchmark is running
   - Cancel the run at any time

Use `Quick DNSSEC Check` to run DNSSEC checks on known resolvers.

Quick DNSSEC uses the same server selection as the form:

- `Nameservers` list
- `Include global DNS providers`
- `Include regional DNS services`

Disable both checkboxes to test only manually entered servers.

Manual nameserver entries that are private/local (for example `192.168.x.x`, `10.x.x.x`, `127.0.0.1`) are ignored automatically.
Benchmark start requires explicit consent for Chromium history usage.
System-discovered private/local resolvers are also ignored automatically.
ASN/country enrichment is disabled by default and uses third-party metadata services only when you explicitly enable it.

## Notes

- History sampling scans Chromium browser profiles (Chrome, Edge, Brave) when available.
- If Chrome history has no eligible external hostnames, benchmark falls back to a built-in public domain set.
- Each benchmark runs twice per hostname to compare cold-cache and warm-cache resolver behavior.
- If no valid history records are found, the API returns warnings and empty results.

## Release notes

- See [RELEASE_NOTES_v0.2.5.md](RELEASE_NOTES_v0.2.5.md)
- Previous: [RELEASE_NOTES_v0.2.4.md](RELEASE_NOTES_v0.2.4.md), [RELEASE_NOTES_v0.2.3.md](RELEASE_NOTES_v0.2.3.md), [RELEASE_NOTES_v0.2.2.md](RELEASE_NOTES_v0.2.2.md), [RELEASE_NOTES_v0.2.1.md](RELEASE_NOTES_v0.2.1.md), [RELEASE_NOTES_v0.2.0.md](RELEASE_NOTES_v0.2.0.md)

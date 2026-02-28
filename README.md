# namebench (modernized fork)

namebench benchmarks DNS resolvers against sampled real-world hostnames from browser history, then ranks servers by latency and reliability.

## Current status

This fork adds:

- Modern responsive web UI
- Multi-server benchmark ranking
- Score table (rank, avg, p95, fail rate, success/fail)
- Visual charts (latency, error rate, winner distribution)
- DNSSEC quick-check endpoint

## Requirements

- Go 1.22+
- C compiler toolchain for `github.com/mattn/go-sqlite3` (CGO dependency)

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
./namebench -port 8080

# Windows
.\\namebench.exe -port 8080
```

By default it auto-opens your browser. If browser auto-open is blocked, open manually:

- `http://127.0.0.1:8080/`

You can disable auto-open:

```bash
./namebench -port 8080 -open_browser=false
```

## How to use

1. Enter one or more DNS servers in the `Nameservers` box (one per line).
2. Optionally enable global/regional provider pools.
3. Set `Number of queries`.
4. Click `Start Benchmark`.
5. Review:
   - Winner banner
   - Ranked results table
   - Latency/Error charts
   - Winner latency distribution chart

Use `Quick DNSSEC Check` to run DNSSEC checks on known resolvers.

## Notes

- History sampling currently uses Chrome history.
- UI includes some forward-looking fields (for future expansion).
- If no valid history records are found, the API returns warnings and empty results.

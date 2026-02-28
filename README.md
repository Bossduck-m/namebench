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

The app uses a random local port by default (`-port=0`) and opens your browser automatically.

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

Quick DNSSEC uses the same server selection as the form:

- `Nameservers` list
- `Include global DNS providers`
- `Include regional DNS services`

Disable both checkboxes to test only manually entered servers.

## Notes

- History sampling currently uses Chrome history.
- UI includes some forward-looking fields (for future expansion).
- If no valid history records are found, the API returns warnings and empty results.

## Release notes

- See [RELEASE_NOTES_v0.2.1.md](RELEASE_NOTES_v0.2.1.md)
- Previous: [RELEASE_NOTES_v0.2.0.md](RELEASE_NOTES_v0.2.0.md)

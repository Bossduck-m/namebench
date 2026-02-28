# namebench v0.2.0 (Modernized Fork)

Date: 2026-03-01

## Highlights

- Modern responsive web UI
- Multi-server DNS benchmark with ranking
- Score table (rank, score, avg, p95, fail rate)
- Visual charts:
  - Average latency by server
  - Error rate by server
  - Winner latency distribution
- Embedded UI assets (binary runs from any folder)
- Browser auto-open support (`-open_browser=true` by default)
- Pure-Go SQLite driver (`modernc.org/sqlite`) to avoid CGO compiler setup

## DNSSEC behavior update

- `Quick DNSSEC Check` now uses the selected servers from the current form.
- If no servers are selected/sent, it falls back to a default resolver set.
- Response log now starts with `checked_servers=<N>`.

## Operational notes

- Benchmark mode uses Chrome history sampling.
- `Include global DNS providers` and `Include regional DNS services` increase the tested server count.
- To test only manually entered servers, disable both provider checkboxes.

## Running

```bash
go mod tidy
go build -o namebench.exe .
./namebench.exe -port 8080
```

Open: `http://127.0.0.1:8080/`

# namebench v0.2.1

Date: 2026-03-01

## What's new

- `Quick DNSSEC Check` now uses the selected servers from the form instead of always using a fixed 4-server list.
- Added Windows one-click build+launch script: `build_windows_gui.bat`.
- Added GitHub Actions workflow to produce a downloadable Windows EXE artifact.
- Default runtime port changed to random local port (`-port=0`) to avoid port-8080 conflicts on double-click launch.

## Why this matters

- You can test exactly the nameservers you entered.
- End users can run the app by double-clicking the generated EXE without typing commands in PowerShell.

# namebench v0.2.5

Date: 2026-03-10

## Improvements

- Expanded public resolver pools with more well-known DNS providers (Google, Cloudflare, Quad9, OpenDNS, AdGuard, etc.).
- Updated default nameserver list in UI to public resolver defaults.
- Added automatic filtering for private/local resolver IPs:
  - `192.168.x.x`
  - `10.x.x.x`
  - `172.16.x.x - 172.31.x.x`
  - `127.x.x.x`
  - link-local and other non-public ranges
- Added a single-instance guard so re-launching opens the already running app instead of starting a second copy.
- Added session-aware idle shutdown so the local backend exits shortly after the last UI tab/window closes.
- Added hidden command helpers on Windows to reduce console flashes during browser launch and resolver discovery.
- Added a randomized per-launch local UI base path to reduce predictable localhost probing.
- Limited benchmark execution to one active run at a time.
- Reduced sensitive local logging around browser history path handling and tightened local state/log file permissions.

## UX updates

- When manual nameservers include invalid/private entries, response warnings now indicate that some entries were ignored.
- Added summary cards for recommended, fastest cold, fastest warm, and most stable resolvers.
- Added a draggable layout splitter and a collapsible result log.
- DNSSEC checks are now POST-only and continue to use the currently selected resolver set.

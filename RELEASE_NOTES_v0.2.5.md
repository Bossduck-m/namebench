# namebench v0.2.5

Date: 2026-03-01

## Improvements

- Expanded public resolver pools with more well-known DNS providers (Google, Cloudflare, Quad9, OpenDNS, AdGuard, etc.).
- Updated default nameserver list in UI to public resolver defaults.
- Added automatic filtering for private/local resolver IPs:
  - `192.168.x.x`
  - `10.x.x.x`
  - `172.16.x.x - 172.31.x.x`
  - `127.x.x.x`
  - link-local and other non-public ranges

## UX updates

- When manual nameservers include invalid/private entries, response warnings now indicate that some entries were ignored.

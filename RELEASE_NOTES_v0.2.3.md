# namebench v0.2.3

Date: 2026-03-01

## Fixes

- Fixed form submit parsing reliability for `Start Benchmark` and `Quick DNSSEC Check`.
- Switched frontend POST payloads to `application/x-www-form-urlencoded` for stable field parsing.
- Improved warning messages:
  - Distinguishes between "no nameserver entered" and "entered nameservers could not be parsed".

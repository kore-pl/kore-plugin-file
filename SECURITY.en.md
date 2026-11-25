# Security Vulnerability Disclosure

This file describes the vulnerability disclosure process for this plugin repository (`kore-plugin-file`); it is not a security design document.

## Scope

Covers: the implementation of the file management plugin (id `file`), the `info.json` capability declarations, any frontend, and `make dist` artifacts.

Not covered: kernel and protocol issues — report those via the [kore SECURITY.md](https://github.com/kore-pl/kore/blob/main/SECURITY.md); known vulnerabilities in third-party dependencies should also be reported upstream.

## Supported versions

| Version | Status | Notes |
| --- | --- | --- |
| `0.3.5` (latest) | ✅ Accepted and fixed | Security fixes are provided only for the latest patch version |
| Earlier `0.x` versions | ❌ Not supported | Please upgrade to the latest version first |

During 0.x we keep a minimal support surface: only the latest patch of each minor receives fixes; older patches are not backported.

## Attack surface in this repository

- **RPC methods**: all `file.*` methods (see `README.md`); missing path validation can lead to traversal, privilege escalation, or data loss.
- **capabilities**: `info.json.capabilities` is the full authorization surface; every extra declaration adds an attack path.
- **Separate process and UDS**: the plugin runs as its own process; its socket is created by the host, `chown`ed to `kore-<id>`, mode `0600`; bypassing socket permissions or connecting to another plugin's socket is a vulnerability.
- **Host write surface**: the controlled root directory, trash, symlink handling, archive extraction, and path validation are security boundaries; writes outside the granted capabilities are vulnerabilities.
- **Frontend**: any web UI runs in an isolated-origin iframe and communicates with the host via postMessage; XSS, message forgery, or cross-plugin data reads are security boundary issues.

## Reporting

1. **Preferred: GitHub Security Advisory** — "Report a vulnerability" on this repository's Security page (private channel, no public issue).
2. **Fallback email**: `pengtang1024@users.noreply.github.com`, subject `[kore security] file`.
3. **Do not open public issues, and do not post PoCs in discussions or PRs.**
4. Include: affected version, reproduction steps or PoC, impact assessment, whether it is public, and attribution preference.

## Timeline and disclosure

- Acknowledge with an initial assessment within **72 hours**; share an investigation plan for complex issues.
- Coordinate disclosure after a fix, default **90-day** window; consult first if exploitation in the wild requires earlier disclosure.
- Record security fixes in `CHANGELOG.md` without exploitation details, and publish a security advisory as needed.

## Acknowledgements

With consent, credit contributors in the release notes; anonymous on request.

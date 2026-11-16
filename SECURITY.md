# Security

## What Claude Monitor touches

- **Reads.** It reads the Claude Code transcripts under `~/.claude/projects`, but only the `usage`, `model`, `timestamp`, `cwd` and `sessionId` fields. It never reads prompt text or tool output.
- **Writes.** It writes to `~/.claude/monitor/` (directory `0700`, files `0600`) and to `~/.claude/settings.json`. In `settings.json` it adds only its own hooks and `statusLine`, makes a backup next to the file, and keeps your file's permissions.
- **Network.** It makes no outbound connections, apart from the installers downloading the release from GitHub. The dashboard listens on `127.0.0.1` only.

## Local server hardening

| Threat | Mitigation |
|---|---|
| A website reading your data (DNS rebinding) | Requests whose `Host` is not `127.0.0.1`, `localhost` or `::1` are rejected |
| A website triggering actions (CSRF) | Actions require the `X-Claude-Monitor` header, which a cross-origin page cannot send without a CORS preflight that is never approved |
| A website framing the panel to trick clicks | `X-Frame-Options: DENY` and CSP `frame-ancestors 'none'` |
| Script injection through project or session names in the panel | All dynamic text is HTML-escaped. The CSP blocks external scripts |
| Shell injection through folder names | Commands run with argument vectors, never through a shell. Folder names are escaped for `wt.exe` (`;`). Notification text is stripped of PowerShell/XML metacharacters, because the Windows toast fallback renders it inside a PowerShell script |

## Known limitations

- **Other local users.** On a shared machine, other users of the same computer can reach `127.0.0.1:47821`. They could read your session names, paths and costs, and ask the app to focus a session or open its folder in your desktop session. They cannot run arbitrary commands through it. If this matters to you, do not run it on multi-user hosts.
- **Unsigned binaries.** Release binaries are not code-signed yet, so Windows SmartScreen and macOS Gatekeeper may warn. `checksums.txt` lets you verify the integrity of a download. It does not prove who built it, because the checksums come from the same release.
- **`curl | sh`.** The one-line installers run a script from this repository. You can read [`scripts/install.sh`](scripts/install.sh) / [`scripts/install.ps1`](scripts/install.ps1) first, or download a binary from Releases and run `claude-monitor install` yourself.
- **Wrapped status line.** If you already had a `statusLine`, Claude Monitor keeps running that same command. It runs with the same trust it had before.

## Reporting a vulnerability

Please open a [private security advisory](https://github.com/GuillermoParedes/claude-monitor/security/advisories/new) instead of a public issue.

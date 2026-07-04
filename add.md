# add.md — design notes and roadmap (for me to keep improving this project)

This file is not user documentation — it's my own working notes so that the
next time I touch this repo, I pick up the same design language instead of
reinventing it. Update this file whenever a new page/pattern is added or a
plan changes; treat it as append/edit, not write-once.

## Current state (as of this writing)

Pages wired into `App.tsx`'s sidebar: **Servers** (dashboard, `pages/Dashboard.tsx`
+ `components/ServerList.tsx`), **Nodes** (`pages/Nodes.tsx`), **Settings**
(`pages/Settings.tsx` — version + update check). **Activity** is a static,
unwired nav item — placeholder for the next page to build.

Backend REST surface: auth (login/me), nodes (list/create, admin-gated),
servers (list/get/power), version (get/check-update), WS gateway for
console/stats (`/ws/servers/{uuid}`, not yet driving any UI beyond the
resource bars on server cards).

Installer (`install.sh` + `scripts/*.sh`): language select (EN/RU with
real explanations, not just translated strings), Docker/Postgres/Redis
provisioning, domain + Let's Encrypt, interactive admin bootstrap, node
install with a non-interactive fast path (`WINGSD_DAEMON_TOKEN=... bash
<(curl ...)`), update mechanism (`PANEL_UPDATE=1 ./install.sh`), full
destructive uninstall gated behind typing `DELETE`.

## Design conventions — follow these before inventing new patterns

1. **Never invent new CSS.** `frontend/src/styles/panel.css` is the design
   system; it was handed to me finished and I don't touch it. Every new page
   is built by finding the closest existing section in panel.css and reusing
   its classes verbatim, even if the semantic match is a little loose (e.g.
   the Nodes table reuses `.db-table/.db-head/.db-row` — "database list"
   markup — because it's the right shape of table, not because nodes are
   databases). If a new page truly needs a shape nothing in panel.css
   covers, that's the one exception where adding a small amount of new CSS
   in a *separate* file is acceptable — but check twice first.
2. **Copyable-command pattern**, established in Nodes/Settings: when an
   action needs to happen on a *different* machine than the browser (install
   a node, run an update), the UI's job is to show a single copy-pastable
   shell command (`.api-item` + `.api-key` + a `.btn-sm` "Copy" button using
   `navigator.clipboard`), not to try to execute it remotely. The panel
   process runs as `www-data` with no privilege to restart its own systemd
   unit or touch Docker on other hosts — don't build a "click to apply"
   button that secretly shells out; it can't have the permissions to do that
   safely, and giving it those permissions is a bigger security decision
   than a UI ticket.
3. **New sidebar page checklist** (do all four or the page is orphaned):
   add the `View` union type in `App.tsx`, add the `nav-item` with
   `onClick={() => goTo('x')}` and the `active` class ternary, add the
   render branch in `<main>`, and reset `activeServer` in `goTo()` if the
   page has no concept of a "current server" (copy the existing pattern,
   don't rewrite it).
4. **i18n pattern** (installer only, not the frontend yet): all
   user-facing *explanatory* strings go through `scripts/i18n.sh`'s
   `MSG_EN`/`MSG_RU` tables and `msg()`. Plumbing/log lines (`log_ok`,
   `log_step`) stay English-only — i18n is for the handful of steps where a
   Russian operator genuinely needs the "why", not for every log line.
5. **Non-interactive fast paths**: every interactive installer prompt should
   have an env-var bypass (`WINGSD_DAEMON_TOKEN`, `PANEL_UPDATE`) so the
   website can eventually generate a single copy-paste command for it. When
   adding a new interactive step, ask "would a copy-paste command from the
   website want to skip this?" — if yes, add the env var check at the same
   time, not as a follow-up.
6. **No comments in code.** This was an explicit, deliberate instruction
   from the project owner, applied retroactively across the whole codebase
   (Go, TS, SQL, proto, bash). Keep following it for new code: no `//`, `#`,
   `--`, or `/* */` explanatory comments anywhere in source files. This
   file and other `.md` docs are the exception — comments belong in design
   notes, not in code.
7. **Build/version discipline**: `scripts/panel.sh`'s `build_panel_binaries`
   embeds `commit`/`buildDate` via `-ldflags -X main.commit=... -X
   main.buildDate=...` into `cmd/panel`. If another binary ever needs to
   report its own version (wingsd, for instance), wire it the same way
   rather than inventing a second mechanism.

## Roadmap — rough priority order

### Near-term (obvious next pages, CSS already exists and is unused)
- **Per-server view** (click "Manage" on a server card — `onManage` already
  wired in `ServerCard`/`Dashboard`/`App`, but there's no destination yet).
  panel.css has a full tab-bar design for this: `.tab-bar`/`.tab-btn` +
  `.tab-panel`, with dedicated sections already styled — Console
  (`.console-wrap`, xterm-like output, wire to `/ws/servers/{uuid}` which
  the backend hub already relays), Files (`.files-table`, needs the daemon's
  file-manager RPCs — not implemented on wingsd yet, see docs/PROTOCOL.md
  §2 "File manager"), Databases (`.db-table`, backend has `server_databases`
  table but no handler), Schedules (`.sch-list`/`.sch-card`/`.toggle-sw`,
  backend has `server_schedules`/`schedule_tasks` tables but no handler
  or cron runner).
- **Activity page** — the sidebar nav item already exists and does nothing.
  panel.css has `.act-table`/`.act-row` ready. Backend has an
  `activity_logs` table but nothing writes to it yet — writing activity
  log rows needs to happen at the point of action (login, power actions,
  node creation), not as an afterthought bolted onto the table later.
- **Account page** — `.acc-grid`/`.acc-card`, `.api-list`/`.api-item` (API
  keys — backend `api_keys` table exists, no handler), `.twofa-card` (2FA —
  `users.totp_secret`/`totp_enabled` columns exist, unused).

### Mid-term (real functionality gaps, not just missing UI)
- Wire the daemon's actual `CreateServer`/`Power` flow end to end: right
  now `ServerHandler` calls `NodeClient(nodeID)`, but `cmd/panel/main.go`'s
  `NodeClient` resolver is a stub that always errors — there's no
  create-server flow in the UI at all yet (no "Add Server" button anywhere,
  no egg/template picker). This is the single biggest gap between "looks
  like Pterodactyl" and "works like Pterodactyl."
  - Bootstrap resolving nodeID -> daemonclient.Client needs a secrets store
    decision (see `docs/ARCHITECTURE.md` security section) before it can be
    wired for real — don't just inline the raw token in Postgres next to
    its own bcrypt hash defeats the point of hashing it.
- RBAC is currently binary (`is_admin` or nothing) — `auth.PermissionChecker`
  interface exists in `backend/internal/auth/rbac.go` but has zero
  implementations wired into the router. `server_subusers` table exists
  for per-server sharing but nothing reads it.
- gRPC migration for the daemon protocol (proto file is complete,
  `daemonclient`/`daemon/internal/api` are still the HTTP/WS stand-in) —
  low urgency, only matters once file-manager/backup streaming RPCs need
  the bidirectional-stream ergonomics gRPC gives you for free.

### Later / polish
- Frontend i18n (the installer got RU/EN; the SPA itself is still
  English-only — if the Russian-speaking installer experience matters,
  the dashboard probably should too, eventually).
- Refresh tokens (`auth.AccessTokenTTL` is 15 minutes, no refresh flow —
  users get silently kicked to the login screen on expiry via the 401
  handler in `api/client.ts`; fine for now, annoying at scale).
- SFTP server on wingsd (mentioned in the original spec, not started).

## Things I keep having to re-explain to myself — write them down once

- `scripts/database.sh`'s `wait_for_postgres` exists because of a real
  incident: VPS images built from container templates ship
  `/usr/sbin/policy-rc.d` that silently blocks `postgresql-16`'s postinst
  from creating a cluster. `neutralize_policy_rc_d` (in `lib.sh`) fixes the
  root cause; `wait_for_postgres`'s cluster-creation fallback is defense in
  depth for hosts that were already broken before that fix existed. Don't
  "simplify" either of these away — they're both load-bearing.
- The frontend never had a login screen until a user hit a live 401 in
  production and asked "why doesn't anything work" — the lesson: when
  scaffolding a new page that hits an authenticated endpoint, check there's
  actually a way to get a token into `localStorage` before calling it done.
- `install.sh`'s self-clone-and-exec only fires when `scripts/lib.sh` isn't
  next to it (i.e. running via `bash <(curl ...)` with nothing checked out
  locally) — don't add prompts before that check runs, they'd never be
  reached in the one-liner case since the script re-execs itself immediately.

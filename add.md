# add.md — design notes and roadmap (for me to keep improving this project)

This file is not user documentation — it's my own working notes so that the
next time I touch this repo, I pick up the same design language instead of
reinventing it. Update this file whenever a new page/pattern is added or a
plan changes; treat it as append/edit, not write-once.

## Current state (as of this writing)

Pages wired into `App.tsx`'s sidebar: **Servers** (dashboard,
`pages/Dashboard.tsx` + `components/ServerList.tsx`, now with a
**"+ Create Server" form** — `components/CreateServerForm.tsx` — that
actually creates a container on the daemon), **Nodes** (`pages/Nodes.tsx`,
now also manages **Allocations** per node), **Activity**
(`pages/Activity.tsx` — real log, not a placeholder), **Settings**
(`pages/Settings.tsx` — version + update check), **Account**
(`pages/Account.tsx` — API key management; reachable from the sidebar or
by clicking the user chip in the topbar). Clicking "Manage" on a server
card goes to **`pages/ServerView.tsx`**, tab-bar with
Overview/Console/Files/Databases/Schedules — Overview (power buttons +
live CPU/RAM/Disk meters) and Console (real, bidirectional) are wired;
Files/Databases/Schedules are honest "not implemented yet" panels, not
fake UI.

Backend REST surface: auth (login/me), nodes (list/create, admin-gated),
servers (list/get/power — power is now genuinely wired end to end, see
below), version (get/check-update). Two WS gateways, both now require
`?token=<jwt>` on the handshake (closed a real gap — they were previously
unauthenticated entirely, since browsers can't set an `Authorization`
header on a WS upgrade and nothing had filled that in yet): `/ws/servers/{uuid}`
relays live stats (polls the daemon every 2s while at least one browser is
subscribed), `/ws/servers/{uuid}/console` relays the daemon's console
bidirectionally (dials `wingsd`'s own `/ws/servers/{uuid}` on first
subscriber, closes it when the last browser leaves — same lazy pattern as
stats, see convention 10 below, now generalized to two independent
room/session maps in the same `Hub`).

**Power actions and live stats are real now, not stubbed.** The chain that
makes this work: `NodeHandler.Create` encrypts the raw daemon token
(AES-256-GCM, key = SHA-256 of `PANEL_ENCRYPTION_KEY`, see
`internal/crypto/aesgcm.go`) into `nodes.daemon_token_encrypted` alongside
the existing bcrypt hash (hash stays for future daemon-side auth
verification if that direction is ever added; encrypted copy is what lets
the *panel* re-authenticate outbound to wingsd). `cmd/panel/main.go`'s
`nodeClientResolver` decrypts it per-call and builds a real
`daemonclient.Client`. `daemon/internal/api/handlers.go` grew a
`GET /servers/{uuid}/stats` endpoint (CPU % computed from the standard
cpu-delta/system-delta/online-cpus formula, docker one-shot stats mode).
`ws.Hub` grew a lazy per-server poller (`FetchStats` callback + a
`pollers map[uuid.UUID]context.CancelFunc`) instead of a global ticker
over every server, so idle servers cost nothing.

**Migrations are now a real numbered system**, not a single re-run file.
`backend/migrations/000N_*.sql`, tracked in a `schema_migrations` table,
applied via `scripts/database.sh`'s `apply_migrations` — called both from
fresh installs (`provision_database`) and from `write_panel_env`/`run_update`
on existing installs (previously, `write_panel_env` returned immediately
if `panel.env` already existed, which meant **schema changes silently
never reached already-installed panels** — caught this while adding
migration `0002`, backfilled a bootstrap check so existing databases with
the old single-file schema and no `schema_migrations` row get `0001`
marked as already-applied instead of the migration runner trying to
`CREATE TABLE users` again and failing).

Installer (`install.sh` + `scripts/*.sh`): language select (EN/RU with
real explanations, not just translated strings), Docker/Postgres/Redis
provisioning, domain + Let's Encrypt, interactive admin bootstrap, node
install with a non-interactive fast path (`WINGSD_DAEMON_TOKEN=... bash
<(curl ...)`), update mechanism (`PANEL_UPDATE=1 ./install.sh`, now also
runs `apply_migrations`), full destructive uninstall gated behind typing
`DELETE`.

**Login now survives longer than 15 minutes.** `auth.TokenManager` issues
two JWTs at login: a short-lived access token (`AccessTokenTTL`, unchanged
at 15m) and a long-lived refresh token (`RefreshTokenTTL`, 30 days — the
config field existed since the original scaffold but was never actually
issued or consumed until now). Both carry a `type` claim (`"access"` |
`"refresh"`); `auth.Middleware` and `authenticateWS` both now reject
anything that isn't `type=access`, so a leaked/long-lived refresh token
can't be used to hit the API directly — it can only be exchanged at
`POST /auth/refresh` for a fresh access+refresh pair. Frontend:
`api/client.ts`'s `request()` catches a 401, calls `/auth/refresh` once
(de-duplicated across concurrent callers via a module-level
`refreshInFlight` promise so five simultaneous 401s don't fire five
refresh calls), retries the original request, and only forces a real
logout if the refresh itself fails (refresh token expired or revoked).

**The create-server flow exists now — the gap called out below as "the
single biggest gap" is closed**, at least for a first working version:
`backend/migrations/0003_seed_eggs.sql` seeds two starter eggs (a generic
Ubuntu container and `itzg/minecraft-server`); `EggHandler`/
`AllocationHandler`/`ServerHandler.Create` are new; the whole creation
happens inside one DB transaction that only commits *after* the daemon
confirms the container was actually created (rolls back the server row
and any claimed allocation otherwise, so a daemon failure never leaves a
ghost server behind). Along the way, fixed a real bug in
`daemon/internal/docker/manager.go`: container creation always overrode
`Cmd` with `/bin/sh -c "<startup_command>"`, even when empty — which
silently broke any egg (like `itzg/minecraft-server`) that relies on the
image's own `ENTRYPOINT`. Empty `StartupCommand` now leaves `Cmd` nil so
Docker falls through to the image's default. Allocations have no
provisioning UI beyond a bare-bones "add one manually" form on the Nodes
page (IP + port) — there's still no concept of a port range, reservation,
or auto-assignment; see roadmap.

**Activity logging** (`internal/activity`) is now called at the three
places that mattered most: login, node creation, server power actions,
and server creation. It intentionally swallows its own errors (logs to
stderr, never fails the calling request) — an activity-log write failing
is not a reason to fail the user's actual action.

**API keys are now a real auth method, not just CRUD.** `auth.Middleware`
takes a second argument, `auth.APIKeyResolver` — if the bearer token has
the `panel_` prefix (how raw API keys are formatted, see
`APIKeyHandler.Create`), it's sha256-hashed and looked up in `api_keys`
instead of being parsed as a JWT; a hit updates `last_used_at` and
produces the same `*auth.Claims` shape a JWT would (with
`Type: auth.TokenAccess`, so it passes the same access-only check).
WS auth (`authenticateWS`) deliberately was **not** extended to accept
API keys — those are for programmatic REST clients, not the browser SPA,
and `tm.Parse` on a `panel_...` string just fails as not-a-JWT, which is
the correct behavior there.

**Server deletion is wired end to end**: `ServerHandler.Delete` checks
owner-or-admin, calls the daemon's `DeleteServer`, then deletes the row
(FKs cascade/null out `server_subusers`/`server_variables`/`allocations.
server_id` automatically). Supports `?force=true` for admins to drop the
DB record even if the node/daemon is unreachable (e.g. a decommissioned
node) — without it, a daemon-side failure blocks the delete rather than
silently orphaning a running container. Frontend: a `.danger-card` in
ServerView's Overview tab, `window.confirm` before calling it (no custom
modal system exists yet, and this is a one-off enough action that
building one wasn't justified).

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
8. **Secrets that the panel needs to use later (not just verify) get
   encrypted, not hashed.** `daemon_token_hash` (bcrypt) proves someone
   presented the right token; it can never answer "what was the token" —
   that's what `daemon_token_encrypted` (AES-GCM via `internal/crypto`,
   keyed off `PANEL_ENCRYPTION_KEY`) is for. Before adding another secret
   column, ask which of these two questions the code actually needs
   answered later.
9. **Any new SQL schema change is a new numbered file in
   `backend/migrations/`, never an edit to `0001_init.sql`.** The migration
   runner tracks applied files in `schema_migrations`; editing an old file
   in place means already-deployed panels never see the change (they've
   already recorded that filename as applied). Bump the number instead.
10. **Live per-server data (stats, console) is relayed lazily, per room.**
    `ws.Hub` only starts polling/dialing for a given server once the first
    browser subscribes to its room, and tears the upstream connection down
    the moment the last one disconnects — `pollers`/`consoleSessions`
    (both `map[uuid.UUID]context.CancelFunc`-shaped) in `internal/ws/hub.go`.
    Don't build a global "poll/dial every server all the time" loop — most
    servers have nobody watching at any given moment. When adding a third
    kind of live relay, add a third independent room+session map rather
    than trying to generalize the first two into one abstraction
    prematurely — stats (HTTP poll) and console (persistent WS dial) have
    different enough connection lifecycles that forcing one interface over
    both cost more than the duplication would.
11. **WS auth is a `?token=<jwt>` query param, not a header.** Browsers
    cannot set `Authorization` on a WebSocket upgrade request, so
    `auth.Middleware` (header-based) doesn't apply to `/ws/*` routes — they
    validate the query param directly (`authenticateWS` in `api/router.go`)
    instead of being wrapped in the same middleware group as the REST API.
    Every new `/ws/*` route needs this same explicit check; it will not be
    caught by putting the route inside `r.Group(auth.Middleware(...))`.
12. **Two-token auth: access is short-lived and API-usable, refresh is
    long-lived and single-purpose.** A JWT's `type` claim (`auth.TokenType`)
    is what enforces this — `auth.Middleware`/`authenticateWS` both check
    `claims.Type == auth.TokenAccess` and reject refresh tokens outright.
    If a future change ever needs a third token flavor (e.g. a one-time
    WS "ticket" instead of putting the access token in the URL), extend
    the same `type` claim rather than adding a parallel token system.
13. **Mutating handlers that change state someone would want an audit
    trail for call `activity.Record` inline, at the same call site, not as
    a follow-up pass.** It was added retroactively once already (see
    the "Activity page" note below on why it was deferred the first
    time — don't repeat that mistake for the next mutating endpoint).
    `activity.Record` never returns an error to the caller; a logging
    failure must never fail the actual request.
14. **A DB write that depends on an external call succeeding (the daemon,
    in `ServerHandler.Create`) goes inside one `pgx.Tx`, and the external
    call happens *before* `tx.Commit()`, not after.** If the daemon call
    fails, `defer tx.Rollback(ctx)` undoes the server row and any claimed
    allocation automatically. Don't insert-then-call-then-cleanup-on-error
    manually — the deferred rollback pattern is shorter and can't leak a
    half-created row if a future code path adds an early return.

## Roadmap — rough priority order

### Near-term (Server Detail's remaining tabs — Overview + Console are done)
- **Files tab** — needs daemon file-manager RPCs first (list/read/write/
  delete/rename over HTTP or the proto's streaming RPCs — see
  docs/PROTOCOL.md §2), then the `.files-table` UI. Biggest remaining lift
  in Server Detail.
- **Databases tab** — `server_databases` table exists, no handler. Needs a
  decision on how DB credentials actually get provisioned (a MySQL/Postgres
  instance per node? shared? the schema has `database_host_id` pointing at
  a `database_hosts` table that was deliberately never created — "out of
  scope v1" per the migration's own note).
- **Schedules tab** — `server_schedules`/`schedule_tasks` tables exist, no
  handler, no cron runner. The runner is the real work here (some process
  needs to wake up and check `cron_minute`/`cron_hour`/etc against wall
  clock and fire `schedule_tasks` in `sequence_id` order) — the UI is the
  easy 10%.
- **2FA** — `.twofa-card` exists in panel.css, `users.totp_secret`/
  `totp_enabled` columns exist, nothing reads or writes them. Account
  page currently only has the API-keys card.
- **Allocations need real provisioning**, not a bare IP+port text form —
  no port-range/bulk-add, no validation that the IP actually belongs to
  the node, no reservation system. Fine for one operator manually running
  a handful of servers; won't scale past that.

### Mid-term (real functionality gaps, not just missing UI)
- RBAC is currently binary (`is_admin` or nothing) — `auth.PermissionChecker`
  interface exists in `backend/internal/auth/rbac.go` but has zero
  implementations wired into the router. `server_subusers` table exists
  for per-server sharing but nothing reads it. `ServerHandler.Create`
  in particular has no limit checks against anything resembling a quota.
  API keys also have no scoping (`api_keys.permissions` JSONB column is
  unused) — a key currently grants the same access the owning user has,
  full stop.
- gRPC migration for the daemon protocol (proto file is complete,
  `daemonclient`/`daemon/internal/api` are still the HTTP/WS stand-in) —
  low urgency, only matters once file-manager/backup streaming RPCs need
  the bidirectional-stream ergonomics gRPC gives you for free.

### Later / polish
- Frontend i18n (the installer got RU/EN; the SPA itself is still
  English-only — if the Russian-speaking installer experience matters,
  the dashboard probably should too, eventually).
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
- Both WS endpoints were completely unauthenticated until this pass — a
  leftover from the original scaffold, which even said so in a comment
  that got stripped during the no-comments cleanup (the gap itself
  outlived the comment describing it, which is exactly the risk of
  removing "TODO"-shaped comments without also checking whether the TODO
  ever got done). Found it while wiring Console, since sending arbitrary
  console commands with zero auth is a much sharper problem than
  read-only stats were. Worth periodically grepping for other
  `r.Get`/`r.Post` calls registered directly on the base router `r`
  instead of inside the authenticated `r.Group` — that's the tell.
- `docker.Manager.CreateContainer` used to always set `Cmd` to
  `/bin/sh -c "<startup_command>"`, even when `StartupCommand` was `""` —
  which produces `/bin/sh -c ""`, a container that starts and immediately
  exits. This silently broke any egg meant to run via the image's own
  `ENTRYPOINT` (e.g. `itzg/minecraft-server`, which needs `Cmd` left nil
  entirely). Found it while seeding the Minecraft egg for the create-server
  flow — the fix is a three-line `if spec.StartupCommand != ""` guard.
  Worth remembering: "override unless empty" is the wrong default when
  wrapping someone else's Docker image; the image's own defaults are
  usually the point of using it.
- Any "if the env/config file already exists, skip everything" guard
  (`write_panel_env`'s original shape) is a trap for anything that needs to
  run on *every* install, not just the first one — found this the hard way
  with migrations: they were silently skipped on every update because they
  were bolted onto the same early-return as secret generation. When adding
  a new "only do this once" step, ask separately "does this specific piece
  need to happen once, or every time" — don't assume the whole function is
  one atomic once-only unit.

-- Telegram MTProto proxy, deployed the same way as other container plugins
-- (category = 'Plugins' -> installs as an ordinary server on whichever node
-- is flagged as the plugin host, see Plugins.tsx).
--
-- Why not the official telegrammessenger/proxy Docker image: its process
-- always listens on a hardcoded internal port 443, but this codebase's
-- allocation model always maps containerPort == hostPort 1:1
-- (server_handler.go builds PortBindings as port->port for every
-- allocation) -- there's no way to expose an internal-443 container on a
-- different external port here, and only one thing on a node can ever bind
-- host port 443. mtprotoproxy (github.com/alexbers/mtprotoproxy, MIT) is a
-- single-file, pure-stdlib-capable Python script whose listen port is just
-- a config value, so it fits this model the same way every other
-- inline-generated egg does (Streamer Stats Site, the RTMP relay).
--
-- The script (2400+ lines, third-party) isn't embedded inline -- instead
-- it's fetched at container start pinned to an exact commit SHA (not a
-- branch, which could change) and verified against a SHA256 computed from
-- that exact commit before it's ever executed. If GitHub ever served
-- different bytes for that commit (compromise, or the pin going stale
-- because the upstream history was rewritten), the checksum check refuses
-- to run it rather than silently executing unverified code as root.
INSERT INTO eggs (category, name, description, docker_image, startup_command, stop_command)
SELECT 'Plugins', 'Telegram MTProto Proxy',
       'Runs a Telegram MTProto proxy (github.com/alexbers/mtprotoproxy) so your audience can connect to Telegram through this server -- useful wherever Telegram itself is blocked. After installing, add a port allocation matching PROXY_PORT on this server''s Network tab so it''s actually reachable. The shareable tg://proxy link is printed to this server''s console on every start (Console tab) -- the secret is generated once and kept in a file, so it survives restarts unless PROXY_SECRET is set explicitly.',
       'python:3.12-alpine',
       'set -e
mkdir -p /home/container
cd /home/container

PORT="${PROXY_PORT:-443}"
NAME="${USERNAME:-tg}"
TLS_DOMAIN="${TLS_DOMAIN:-www.google.com}"

if [ -n "$PROXY_SECRET" ]; then
  SECRET="$PROXY_SECRET"
elif [ -f .secret ]; then
  SECRET="$(cat .secret)"
else
  SECRET="$(python3 -c "import secrets; print(secrets.token_hex(16))")"
  echo "$SECRET" > .secret
fi

MTPROTOPROXY_COMMIT="bd5b03cd998eee5e916c5fcea8519d6d2c331d4d"
MTPROTOPROXY_SHA256="9ad37e6f86430eb973327d64605d6739890b4a4e2891fa72bc2f2ffb35d62e2"
if [ ! -f mtprotoproxy.py ] || [ "$(sha256sum mtprotoproxy.py | cut -d " " -f1)" != "$MTPROTOPROXY_SHA256" ]; then
  echo "Fetching mtprotoproxy.py (pinned to commit $MTPROTOPROXY_COMMIT)..."
  wget -qO mtprotoproxy.py "https://raw.githubusercontent.com/alexbers/mtprotoproxy/$MTPROTOPROXY_COMMIT/mtprotoproxy.py"
  ACTUAL_SHA256="$(sha256sum mtprotoproxy.py | cut -d " " -f1)"
  if [ "$ACTUAL_SHA256" != "$MTPROTOPROXY_SHA256" ]; then
    echo "mtprotoproxy.py checksum mismatch (expected $MTPROTOPROXY_SHA256, got $ACTUAL_SHA256) -- refusing to run unverified code" >&2
    rm -f mtprotoproxy.py
    exit 1
  fi
fi

cat > config.py <<CONFEOF
PORT = $PORT
USERS = {
    "$NAME": "$SECRET",
}
MODES = {
    "classic": False,
    "secure": False,
    "tls": True,
}
TLS_DOMAIN = "$TLS_DOMAIN"
CONFEOF

if [ -n "$AD_TAG" ]; then
  echo "AD_TAG = \"$AD_TAG\"" >> config.py
fi

apk add --no-cache py3-cryptography >/dev/null 2>&1 || true

echo "Starting Telegram MTProto proxy on port $PORT -- your tg://proxy link will be printed below:"
exec python3 mtprotoproxy.py',
       'exit'
WHERE NOT EXISTS (SELECT 1 FROM eggs WHERE name = 'Telegram MTProto Proxy');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Proxy port (also add a matching port allocation on the Network tab)', 'PROXY_PORT', '443', TRUE, 'required|integer'
FROM eggs WHERE name = 'Telegram MTProto Proxy'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'PROXY_PORT');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Proxy secret (optional, 32 hex chars -- leave blank to auto-generate and keep)', 'PROXY_SECRET', '', TRUE, 'nullable'
FROM eggs WHERE name = 'Telegram MTProto Proxy'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'PROXY_SECRET');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Username label (shown in the proxy link, cosmetic only)', 'USERNAME', 'tg', TRUE, 'required'
FROM eggs WHERE name = 'Telegram MTProto Proxy'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'USERNAME');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'TLS disguise domain (an existing, real domain -- bad probes get proxied there)', 'TLS_DOMAIN', 'www.google.com', TRUE, 'required'
FROM eggs WHERE name = 'Telegram MTProto Proxy'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'TLS_DOMAIN');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Ad tag (optional, from @MTProxybot -- enables Telegram''s official middle-proxy routing)', 'AD_TAG', '', TRUE, 'nullable'
FROM eggs WHERE name = 'Telegram MTProto Proxy'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'AD_TAG');

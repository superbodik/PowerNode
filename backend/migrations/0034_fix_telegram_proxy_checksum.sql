-- Fixes a transcription bug in 0031: the pinned SHA256 for mtprotoproxy.py
-- was stored one hex character short (63 chars instead of 64) -- the
-- downloaded file was always genuinely correct, but the stored expected
-- hash could never match it, so the startup script's own tripwire (refuse
-- to run unverified code on a mismatch) fired on every single boot,
-- crash-looping the container forever. Re-verified fresh against the same
-- pinned commit rather than trusting the earlier transcription: sha256sum
-- of https://raw.githubusercontent.com/alexbers/mtprotoproxy/
-- bd5b03cd998eee5e916c5fcea8519d6d2c331d4d/mtprotoproxy.py is
-- 9ad37e6f86430eb973327d64605d6739890b4a4e2891fa72bc2f2ffb35d62e2d (64 hex
-- chars) -- confirmed by counting, not just eyeballing it this time.
--
-- Only fixes the egg's own template for servers created from here on --
-- an already-created Telegram Proxy server has its own copy of
-- startup_command frozen in the servers table from when it was created, so
-- existing installs need to either be recreated (Plugins page: uninstall,
-- reinstall) or have their startup command edited by hand on the server's
-- Overview tab to match this.
UPDATE eggs
SET startup_command = 'set -e
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
MTPROTOPROXY_SHA256="9ad37e6f86430eb973327d64605d6739890b4a4e2891fa72bc2f2ffb35d62e2d"
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
exec python3 mtprotoproxy.py'
WHERE name = 'Telegram MTProto Proxy';

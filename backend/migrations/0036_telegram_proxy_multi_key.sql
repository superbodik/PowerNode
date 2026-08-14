-- Multiple named keys sharing one port, instead of one key per server.
-- MTProto already authenticates each connection by its own secret, so
-- separate users don't need separate ports for isolation -- giving each
-- key its own port would only mean more allocations and more processes
-- for zero actual benefit. keys.json (a JSON array of {name, secret})
-- replaces the single .secret file; the panel's "create/revoke key"
-- buttons just read-modify-write that file through the existing file API
-- and restart the server, same as any other config change here.
--
-- Backward compatible: a server already running under 0035 has a
-- .secret file but no keys.json yet -- first boot under this script seeds
-- keys.json from that existing secret (plus USERNAME/PROXY_SECRET if
-- still set) instead of minting a fresh one, so links already handed out
-- keep working.
--
-- Also installs uvloop (confirmed real musllinux wheels exist for cp312
-- on PyPI, not assumed) -- mtprotoproxy detects and uses it automatically
-- if present, and it's the one dependency actually worth the pip install
-- given the earlier apk-vs-image-interpreter mismatch. Best-effort: a
-- failed install just means slightly worse throughput, not a crash.
UPDATE eggs
SET startup_command = 'set -e
mkdir -p /home/container
cd /home/container

PORT="${PROXY_PORT:-443}"
TLS_DOMAIN="${TLS_DOMAIN:-www.google.com}"

if [ ! -f keys.json ]; then
  NAME="${USERNAME:-tg}"
  if [ -n "$PROXY_SECRET" ]; then
    SECRET="$PROXY_SECRET"
  elif [ -f .secret ]; then
    SECRET="$(cat .secret)"
  else
    SECRET="$(python3 -c "import secrets; print(secrets.token_hex(16))")"
  fi
  python3 -c "
import json, sys
json.dump([{\"name\": sys.argv[1], \"secret\": sys.argv[2]}], open(\"keys.json\", \"w\", encoding=\"utf-8\"))
" "$NAME" "$SECRET"
fi

MTPROTOPROXY_COMMIT="bd5b03cd998eee5e916c5fcea8519d6d2c331d4d"
MTPROTOPROXY_TARBALL_SHA256="81d3bc9bdaa4cf0d02bc03d65b5b2dd15b3ecfae7a9162df6ccf074e26318588"
if [ ! -f mtprotoproxy.py ] || [ ! -d pyaes ] || [ "$(cat .mtprotoproxy_commit 2>/dev/null)" != "$MTPROTOPROXY_COMMIT" ]; then
  echo "Fetching mtprotoproxy (pinned to commit $MTPROTOPROXY_COMMIT)..."
  wget -qO mtprotoproxy.tar.gz "https://github.com/alexbers/mtprotoproxy/archive/$MTPROTOPROXY_COMMIT.tar.gz"
  ACTUAL_SHA256="$(sha256sum mtprotoproxy.tar.gz | cut -d " " -f1)"
  if [ "$ACTUAL_SHA256" != "$MTPROTOPROXY_TARBALL_SHA256" ]; then
    echo "mtprotoproxy tarball checksum mismatch (expected $MTPROTOPROXY_TARBALL_SHA256, got $ACTUAL_SHA256) -- refusing to run unverified code" >&2
    rm -f mtprotoproxy.tar.gz
    exit 1
  fi
  python3 -c "
import tarfile, sys
with tarfile.open(\"mtprotoproxy.tar.gz\") as tf:
    prefix = sys.argv[1] + \"/\"
    for member in tf.getmembers():
        if not member.name.startswith(prefix):
            continue
        rel = member.name[len(prefix):]
        if rel == \"mtprotoproxy.py\" or (rel.startswith(\"pyaes/\") and rel != \"pyaes/\"):
            if member.isdir():
                continue
            member.name = rel
            tf.extract(member, \".\")
" "mtprotoproxy-$MTPROTOPROXY_COMMIT"
  rm -f mtprotoproxy.tar.gz
  echo "$MTPROTOPROXY_COMMIT" > .mtprotoproxy_commit
fi

python3 -c "
import json, sys
port, tls_domain = sys.argv[1], sys.argv[2]
keys = json.load(open(\"keys.json\", encoding=\"utf-8\"))
users = {k[\"name\"]: k[\"secret\"] for k in keys}
with open(\"config.py\", \"w\", encoding=\"utf-8\") as f:
    f.write(\"PORT = \" + str(int(port)) + chr(10))
    f.write(\"USERS = \" + repr(users) + chr(10))
    f.write(\"MODES = \" + repr({\"classic\": False, \"secure\": False, \"tls\": True}) + chr(10))
    f.write(\"TLS_DOMAIN = \" + repr(tls_domain) + chr(10))
" "$PORT" "$TLS_DOMAIN"

if [ -n "$AD_TAG" ]; then
  python3 -c "
import sys
with open(\"config.py\", \"a\", encoding=\"utf-8\") as f:
    f.write(\"AD_TAG = \" + repr(sys.argv[1]) + chr(10))
" "$AD_TAG"
fi

pip install --quiet --disable-pip-version-check uvloop >/dev/null 2>&1 || true

KEY_COUNT="$(python3 -c "import json; print(len(json.load(open(\"keys.json\", encoding=\"utf-8\"))))")"
echo "Starting Telegram MTProto proxy on port $PORT with $KEY_COUNT key(s) -- links for each print below:"
exec python3 mtprotoproxy.py'
WHERE name = 'Telegram MTProto Proxy';

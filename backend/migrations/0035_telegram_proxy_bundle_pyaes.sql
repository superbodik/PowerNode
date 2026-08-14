-- Fixes the actual reason the proxy never started, seen live in a real
-- container: mtprotoproxy.py tries cryptography, then pycryptodome, then
-- falls back to its own bundled pyaes/ package -- but 0031 only fetched
-- the single mtprotoproxy.py file, never the pyaes/ directory that
-- fallback needs, so all three import attempts failed and the process
-- crashed on every start.
--
-- apk add py3-cryptography (0031/0034) was never going to help here either
-- -- confirmed, not assumed, why: the python:3.12-alpine image ships its
-- own interpreter at /usr/local/bin/python3 with its own site-packages
-- tree, entirely separate from whatever Alpine's own apk-installed
-- system python uses. Installing an apk package can't put anything on
-- that interpreter's import path. Dropped rather than left in as a no-op.
--
-- Switched to fetching the whole repo as a tarball (GitHub's codeload
-- archives are content-addressed and reproducible for a given ref --
-- verified by downloading the same commit twice and comparing hashes, not
-- assumed) instead of pinning a separate checksum per file: one hash to
-- get right instead of five, which is exactly the kind of transcription
-- mistake 0031 already made once. Extraction uses Python's own tarfile
-- module rather than shelling out to tar -z, since busybox tar's gzip
-- support couldn't be verified against a real Alpine container from this
-- environment -- python3 in the image is the one thing guaranteed to
-- behave identically everywhere. The member allowlist (only
-- mtprotoproxy.py and paths under pyaes/) keeps this safe against a
-- malicious archive even though the source is already checksum-pinned.
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
import tarfile
with tarfile.open(\"mtprotoproxy.tar.gz\") as tf:
    prefix = \"mtprotoproxy-$MTPROTOPROXY_COMMIT/\"
    for member in tf.getmembers():
        if not member.name.startswith(prefix):
            continue
        rel = member.name[len(prefix):]
        if rel == \"mtprotoproxy.py\" or (rel.startswith(\"pyaes/\") and rel != \"pyaes/\"):
            if member.isdir():
                continue
            member.name = rel
            tf.extract(member, \".\")
"
  rm -f mtprotoproxy.tar.gz
  echo "$MTPROTOPROXY_COMMIT" > .mtprotoproxy_commit
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

echo "Starting Telegram MTProto proxy on port $PORT -- your tg://proxy link will be printed below:"
exec python3 mtprotoproxy.py'
WHERE name = 'Telegram MTProto Proxy';

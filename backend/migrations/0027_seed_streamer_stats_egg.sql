-- A self-contained stats page for a streamer -- unlike the generic
-- "Python: Website" egg, this one needs no file upload: the whole server
-- is generated inline at container start, same pattern as the RTMP relay
-- egg's mediamtx.yml. Pulls live viewer count + donation totals from the
-- panel's own /streamers/analytics endpoint (already built for the
-- Analytics tile), authenticated with a scoped API key the streamer
-- generates themselves under Account -> API Keys -- no new auth mechanism,
-- reuses what already exists.
INSERT INTO eggs (category, name, description, docker_image, startup_command, stop_command)
SELECT 'streaming', 'Streamer Stats Site',
       'A public stats page (live status, viewer count, donation totals) for your channel -- no file upload needed, works out of the box. Create an API key under Account -> API Keys and paste it into POWERNODE_API_KEY below.',
       'python:3.12-alpine',
       'set -e
mkdir -p /home/container
cat > /home/container/stats_site.py <<PYEOF
import json
import os
import time
import urllib.request
from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler

API_URL = os.environ.get("POWERNODE_API_URL", "").rstrip("/")
API_KEY = os.environ.get("POWERNODE_API_KEY", "")
DISPLAY_NAME = os.environ.get("DISPLAY_NAME", "Streamer")
REFRESH_SECONDS = int(os.environ.get("REFRESH_SECONDS", "15"))

cache = {"data": None, "error": None, "fetched_at": 0.0}

def fetch():
    if not API_URL or not API_KEY:
        cache["error"] = "POWERNODE_API_URL / POWERNODE_API_KEY not set"
        return
    req = urllib.request.Request(
        API_URL + "/streamers/analytics",
        headers={"Authorization": "Bearer " + API_KEY},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            cache["data"] = json.loads(resp.read().decode("utf-8"))
            cache["error"] = None
    except Exception as exc:
        cache["error"] = str(exc)
    cache["fetched_at"] = time.time()

def format_money(totals):
    if not totals:
        return "&mdash;"
    return " + ".join(
        "%.2f %s" % (t["total_cents"] / 100, t["currency"].upper()) for t in totals
    )

def render_body():
    data = cache["data"]
    if data is None:
        return "<p class=err>%s</p>" % (cache["error"] or "Loading...")
    live = data.get("live")
    viewers = data.get("viewer_count", 0)
    today = format_money(data.get("donations_today", []))
    all_time = format_money(data.get("donations_all_time", []))
    status = "LIVE" if live else "OFFLINE"
    status_class = "live" if live else "offline"
    return (
        "<div class=\"status %s\">%s</div>"
        "<div class=grid>"
        "<div class=tile><div class=value>%s</div><div class=label>Viewers</div></div>"
        "<div class=tile><div class=value>%s</div><div class=label>Donations today</div></div>"
        "<div class=tile><div class=value>%s</div><div class=label>Donations all-time</div></div>"
        "</div>"
    ) % (status_class, status, viewers if live else "&mdash;", today, all_time)

def render_page():
    return (
        "<!doctype html><html><head><meta charset=utf-8>"
        "<meta http-equiv=refresh content=%s>"
        "<title>%s stats</title><style>"
        "body{margin:0;background:#0a080c;color:#ece4e8;font-family:-apple-system,Segoe UI,sans-serif;"
        "display:flex;align-items:center;justify-content:center;min-height:100vh}"
        ".card{text-align:center}h1{font-weight:400;color:#e8a8b8}"
        ".status{display:inline-block;padding:6px 16px;border-radius:20px;font-weight:700;margin-bottom:20px}"
        ".status.live{background:rgba(35,165,89,.15);color:#5fe69a}"
        ".status.offline{background:rgba(255,255,255,.08);color:#8a7a82}"
        ".grid{display:flex;gap:16px}"
        ".tile{background:rgba(255,255,255,.03);border:1px solid rgba(232,168,184,.15);border-radius:12px;padding:20px 28px}"
        ".value{font-size:28px;font-weight:700}.label{font-size:11px;color:#8a7a82;margin-top:4px}"
        ".err{color:#f23f43}"
        "</style></head><body><div class=card><h1>%s</h1>%s</div></body></html>"
    ) % (REFRESH_SECONDS, DISPLAY_NAME, DISPLAY_NAME, render_body())

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if time.time() - cache["fetched_at"] > REFRESH_SECONDS:
            fetch()
        html = render_page().encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(html)))
        self.end_headers()
        self.wfile.write(html)

    def log_message(self, fmt, *args):
        print("%s - %s" % (self.address_string(), fmt % args))

if __name__ == "__main__":
    fetch()
    port = int(os.environ.get("PORT", "8000"))
    server = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    print("stats site serving on 0.0.0.0:%d" % port)
    server.serve_forever()
PYEOF
exec python3 /home/container/stats_site.py',
       'exit'
WHERE NOT EXISTS (SELECT 1 FROM eggs WHERE name = 'Streamer Stats Site');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Panel API URL', 'POWERNODE_API_URL', '', TRUE, 'required'
FROM eggs WHERE name = 'Streamer Stats Site'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'POWERNODE_API_URL');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Panel API key (Account -> API Keys)', 'POWERNODE_API_KEY', '', TRUE, 'required'
FROM eggs WHERE name = 'Streamer Stats Site'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'POWERNODE_API_KEY');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Display name', 'DISPLAY_NAME', 'Streamer', TRUE, 'required'
FROM eggs WHERE name = 'Streamer Stats Site'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'DISPLAY_NAME');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Refresh interval (seconds)', 'REFRESH_SECONDS', '15', TRUE, 'required|integer'
FROM eggs WHERE name = 'Streamer Stats Site'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'REFRESH_SECONDS');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Port', 'PORT', '8000', TRUE, 'required|integer'
FROM eggs WHERE name = 'Streamer Stats Site'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'PORT');

import os
import smtplib
from email.mime.text import MIMEText
from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
from urllib.parse import parse_qs

PUBLIC_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "public")

# Contact form config -- all optional, same opt-in shape as the panel's own
# integrations: unset means the form still renders, submissions just fail
# with a clear error instead of silently vanishing.
SMTP_HOST = os.environ.get("WEBSITE_SMTP_HOST", "")
SMTP_PORT = int(os.environ.get("WEBSITE_SMTP_PORT", "587"))
SMTP_USERNAME = os.environ.get("WEBSITE_SMTP_USERNAME", "")
SMTP_PASSWORD = os.environ.get("WEBSITE_SMTP_PASSWORD", "")
SMTP_FROM = os.environ.get("WEBSITE_SMTP_FROM", SMTP_USERNAME)
CONTACT_EMAIL = os.environ.get("WEBSITE_CONTACT_EMAIL", "m4erzforstream@gmail.com")

MAX_BODY_BYTES = 10_000
MAX_FIELD_LEN = 2000


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=PUBLIC_DIR, **kwargs)

    def log_message(self, format, *args):
        print("%s - %s" % (self.address_string(), format % args))

    def do_POST(self):
        if self.path != "/contact":
            self.send_error(404)
            return

        length = int(self.headers.get("Content-Length", 0))
        if length <= 0 or length > MAX_BODY_BYTES:
            self.send_error(400, "Invalid request body")
            return
        body = self.rfile.read(length).decode("utf-8", errors="replace")
        fields = parse_qs(body)

        def field(name):
            return fields.get(name, [""])[0].strip()[:MAX_FIELD_LEN]

        # Honeypot: a real visitor never sees or fills this field (off-screen
        # via CSS). Pretend success without sending anything -- no need to
        # tip off whatever's submitting it.
        if field("website"):
            self._redirect_to_thanks()
            return

        name, contact, message = field("name"), field("contact"), field("message")
        if not name or not contact or not message:
            self.send_error(400, "Missing required field")
            return

        if not SMTP_HOST or not SMTP_USERNAME or not SMTP_PASSWORD:
            print("contact form: WEBSITE_SMTP_* is not configured -- dropping submission")
            self.send_error(503, "Contact form isn't configured on this server yet")
            return

        try:
            self._send_email(name, contact, message)
        except Exception as exc:
            print(f"contact form: failed to send email: {exc}")
            self.send_error(502, "Could not send your message -- try again later")
            return

        self._redirect_to_thanks()

    def _send_email(self, name, contact, message):
        body = f"From: {name}\nReply via: {contact}\n\n{message}"
        msg = MIMEText(body)
        msg["Subject"] = f"PowerNode collab inquiry from {name}"
        msg["From"] = SMTP_FROM
        msg["To"] = CONTACT_EMAIL
        msg["Reply-To"] = contact

        with smtplib.SMTP(SMTP_HOST, SMTP_PORT, timeout=10) as smtp:
            smtp.starttls()
            smtp.login(SMTP_USERNAME, SMTP_PASSWORD)
            smtp.sendmail(SMTP_FROM, [CONTACT_EMAIL], msg.as_string())

    def _redirect_to_thanks(self):
        self.send_response(303)
        self.send_header("Location", "/thanks.html")
        self.end_headers()


def main():
    port = int(os.environ.get("PORT", "8000"))
    server = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    print(f"PowerNode landing page serving {PUBLIC_DIR} on 0.0.0.0:{port}")
    server.serve_forever()


if __name__ == "__main__":
    main()

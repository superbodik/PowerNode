# PowerNode landing page

Static marketing site for PowerNode. Zero third-party dependencies — `app.py`
is a stdlib-only static file server (plus a `/contact` form handler using
only `smtplib`), so it runs on the panel's own "Python: Website" egg with no
`requirements.txt` needed.

## Run locally

```bash
python app.py            # serves ./public on 0.0.0.0:8000
PORT=3000 python app.py  # or pick a port
```

## Deploy on PowerNode itself

1. Create a server using the **Python: Website** egg.
2. Upload everything in this folder (`app.py`, `requirements.txt`, `public/`)
   to the server's `/home/container`.
3. Set the `PORT` environment variable on the server to match whatever port
   the panel allocated for it — the egg's startup command runs
   `python3 ${START_FILE:-app.py}`, and `app.py` reads `PORT` from the
   environment.
4. Start the server.

That's it — no build step, no framework.

## Contact form

The "Want to work together?" form on the homepage POSTs to `/contact`
(plain HTML form, no JS/fetch, so it works with no CORS setup at all) and
emails the submission via SMTP. Set these env vars on the server for it to
actually send anything -- without them, the form still renders but
submissions fail with a clear error instead of silently vanishing:

```
WEBSITE_SMTP_HOST=smtp.gmail.com
WEBSITE_SMTP_PORT=587
WEBSITE_SMTP_USERNAME=your-address@gmail.com
WEBSITE_SMTP_PASSWORD=an-app-password           # not your normal Gmail password
WEBSITE_CONTACT_EMAIL=m4erzforstream@gmail.com  # where submissions land (defaults to this)
```

If using Gmail: regular account passwords don't work for SMTP login anymore --
generate an **App Password** at myaccount.google.com/apppasswords (requires
2-Step Verification turned on) and use that for `WEBSITE_SMTP_PASSWORD`.

Includes a basic honeypot field (`website`, hidden off-screen via CSS) to
drop bot submissions without emailing anything -- not bulletproof, but
enough for a low-traffic contact form.

#!/usr/bin/env bash

PANEL_FQDN=""
PANEL_USES_TLS="no"

prompt_domain() {
	echo
	log_step "Domain"
	echo "$(msg domain_intro)"
	echo

	local subdomain root_domain
	read -rp "$(msg domain_ask_subdomain)" subdomain
	read -rp "$(msg domain_ask_root)" root_domain

	if [[ -z "$root_domain" ]]; then
		log_warn "$(msg domain_skip)"
		PANEL_FQDN=""
		return
	fi

	if [[ -n "$subdomain" ]]; then
		PANEL_FQDN="${subdomain}.${root_domain}"
	else
		PANEL_FQDN="$root_domain"
	fi
	log_ok "Domain: ${PANEL_FQDN}"
}

apply_domain_to_nginx() {
	if [[ -z "$PANEL_FQDN" ]]; then
		return
	fi

	sed -i "s/server_name _;/server_name ${PANEL_FQDN};/" /etc/nginx/sites-available/panel
	systemctl reload nginx 2>/dev/null || systemctl restart nginx

	issue_certificate
}

issue_certificate() {
	if ! require_command certbot; then
		apt-get install -y -qq certbot python3-certbot-nginx || {
			log_warn "Failed to install certbot — continuing on plain HTTP"
			return
		}
	fi

	local cert_email
	read -rp "$(msg cert_email_ask)" cert_email

	log_step "$(msg cert_issuing) ${PANEL_FQDN}"

	local certbot_args=(--nginx -d "$PANEL_FQDN" --non-interactive --agree-tos --redirect)
	if [[ -n "$cert_email" ]]; then
		certbot_args+=(-m "$cert_email")
	else
		certbot_args+=(--register-unsafely-without-email)
	fi

	if certbot "${certbot_args[@]}"; then
		PANEL_USES_TLS="yes"
		log_ok "Certificate installed for ${PANEL_FQDN} (nginx now redirects HTTP -> HTTPS)"
	else
		log_warn "$(msg cert_failed)"
	fi
}

panel_url() {
	local scheme="http"
	[[ "$PANEL_USES_TLS" == "yes" ]] && scheme="https"

	if [[ -n "$PANEL_FQDN" ]]; then
		echo "${scheme}://${PANEL_FQDN}"
	else
		echo "http://$(curl -fs -4 https://ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')"
	fi
}

# Persists the panel's own public address to panel.env as PANEL_PUBLIC_URL,
# once domain/TLS setup (prompt_domain + apply_domain_to_nginx) has already
# run so PANEL_FQDN/PANEL_USES_TLS are settled. Needed for building OAuth
# redirect URIs (Twitch, and anything else later) -- those get redirected
# to by an outside service, so they can't be relative or assume localhost.
write_panel_public_url() {
	local env_file="${PANEL_ENV_FILE:-/etc/panel/panel.env}"
	[[ -f "$env_file" ]] || return
	if grep -q '^PANEL_PUBLIC_URL=' "$env_file"; then
		return
	fi
	local url
	url=$(panel_url)
	echo "PANEL_PUBLIC_URL=${url}" >>"$env_file"
	log_ok "Recorded panel public URL: ${url}"
}

# Same as above but for `run_update` on an already-installed panel, where
# this script invocation never ran prompt_domain/apply_domain_to_nginx (those
# are fresh-install-only), so PANEL_FQDN/PANEL_USES_TLS aren't in scope here.
# Reconstructs the same information from what's already on disk: the domain
# nginx was configured with, and whether certbot issued a certificate for it.
backfill_panel_public_url() {
	local env_file="${PANEL_ENV_FILE:-/etc/panel/panel.env}"
	[[ -f "$env_file" ]] || return
	if grep -q '^PANEL_PUBLIC_URL=' "$env_file"; then
		return
	fi

	local fqdn scheme="http" url
	fqdn=$(sed -n 's/.*server_name \([^;]*\);.*/\1/p' /etc/nginx/sites-available/panel 2>/dev/null | head -1 | awk '{print $1}')

	if [[ -n "$fqdn" && "$fqdn" != "_" ]]; then
		[[ -f "/etc/letsencrypt/live/${fqdn}/fullchain.pem" ]] && scheme="https"
		url="${scheme}://${fqdn}"
	else
		url="http://$(curl -fs -4 https://ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')"
	fi

	echo "PANEL_PUBLIC_URL=${url}" >>"$env_file"
	log_ok "Backfilled panel public URL: ${url}"
}

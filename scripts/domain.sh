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

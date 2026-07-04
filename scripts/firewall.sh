#!/usr/bin/env bash

configure_firewall() {
	log_step "Configuring firewall (UFW)"

	if ! require_command ufw; then
		apt-get install -y -qq ufw || die "UFW installation failed"
	fi

	local ssh_port
	ssh_port=$(ss -tnlp 2>/dev/null | awk '/sshd/ {print $4}' | sed -E 's/.*:([0-9]+)$/\1/' | head -n1)
	ufw allow "${ssh_port:-22}/tcp" comment "SSH"

	ufw allow 80/tcp   comment "Panel HTTP"
	ufw allow 443/tcp  comment "Panel HTTPS"
	ufw allow 2022/tcp comment "SFTP"

	if [[ "${INSTALL_MODE:-}" == "daemon" || "${INSTALL_MODE:-}" == "all" ]]; then
		ufw allow 8443/tcp comment "wingsd control-plane"
	fi

	ufw --force enable
	log_ok "UFW enabled: $(ufw status | tr '\n' ' ')"
}

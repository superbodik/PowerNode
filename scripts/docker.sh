#!/usr/bin/env bash

install_docker() {
	log_step "Installing Docker"

	if require_command docker; then
		log_ok "Docker already installed ($(docker --version))"
	else
		install -m 0755 -d /etc/apt/keyrings
		curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
		chmod a+r /etc/apt/keyrings/docker.asc

		local arch
		arch="$(dpkg --print-architecture)"
		source /etc/os-release
		echo \
			"deb [arch=${arch} signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
			>/etc/apt/sources.list.d/docker.list

		apt-get update -qq
		apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
			|| die "Docker installation failed"

		log_ok "Docker installed ($(docker --version))"
	fi

	systemctl enable --now docker
	log_ok "Docker service enabled and running"
}

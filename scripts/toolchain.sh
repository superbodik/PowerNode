#!/usr/bin/env bash

GO_VERSION="1.22.5"
NODE_MAJOR="20"

install_go() {
	if require_command go; then
		local current
		current=$(go version | awk '{print $3}' | sed 's/go//')
		log_ok "Go already installed (${current})"
		return
	fi

	log_step "Installing Go ${GO_VERSION}"
	local arch tarball
	case "$(uname -m)" in
		x86_64) arch="amd64" ;;
		aarch64) arch="arm64" ;;
		*) die "Unsupported architecture for Go install: $(uname -m)" ;;
	esac
	tarball="go${GO_VERSION}.linux-${arch}.tar.gz"

	curl -fsSL "https://go.dev/dl/${tarball}" -o "/tmp/${tarball}" \
		|| die "Failed to download Go ${GO_VERSION}"
	rm -rf /usr/local/go
	tar -C /usr/local -xzf "/tmp/${tarball}" || die "Failed to extract Go tarball"
	rm -f "/tmp/${tarball}"

	ln -sf /usr/local/go/bin/go /usr/local/bin/go
	ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
	echo 'export PATH=$PATH:/usr/local/go/bin' >/etc/profile.d/go.sh

	export PATH="$PATH:/usr/local/go/bin"

	require_command go || die "Go install completed but 'go' is still not on PATH"
	log_ok "Go installed ($(go version))"
}

install_nodejs() {
	if require_command node; then
		log_ok "Node.js already installed ($(node --version))"
		return
	fi

	log_step "Installing Node.js ${NODE_MAJOR}.x"
	curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - >/dev/null 2>&1 \
		|| die "Failed to configure NodeSource repository"
	apt-get install -y -qq nodejs || die "Node.js installation failed"

	require_command node || die "Node install completed but 'node' is still not on PATH"
	log_ok "Node.js installed ($(node --version), npm $(npm --version))"
}

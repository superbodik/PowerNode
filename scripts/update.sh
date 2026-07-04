#!/usr/bin/env bash

run_update() {
	log_step "Updating from ${REPO_URL} (${REPO_BRANCH})"

	if [[ ! -d "${PROJECT_ROOT}/.git" ]]; then
		die "PROJECT_ROOT (${PROJECT_ROOT}) is not a git checkout — reinstall via the curl one-liner so updates have something to pull from"
	fi

	git -C "$PROJECT_ROOT" fetch --quiet origin "$REPO_BRANCH" \
		|| die "git fetch failed — check network access from this host"
	git -C "$PROJECT_ROOT" reset --hard --quiet "origin/${REPO_BRANCH}" \
		|| die "git reset failed"
	local new_commit
	new_commit=$(git -C "$PROJECT_ROOT" rev-parse --short HEAD)
	log_ok "Checked out ${new_commit}"

	if [[ -x "${PANEL_INSTALL_DIR:-/opt/panel}/panel" ]]; then
		patch_panel_source_dir
		build_panel_binaries
		systemctl restart panel
		log_ok "panel.service restarted on ${new_commit}"
	fi

	if [[ -x "${DAEMON_INSTALL_DIR:-/opt/wingsd}/wingsd" ]]; then
		build_daemon_binary
		systemctl restart wingsd
		log_ok "wingsd.service restarted on ${new_commit}"
	fi

	log_step "Update complete (${new_commit})"
}

patch_panel_source_dir() {
	local env_file="${PANEL_ENV_FILE:-/etc/panel/panel.env}"
	if [[ -f "$env_file" ]] && ! grep -q '^PANEL_SOURCE_DIR=' "$env_file"; then
		echo "PANEL_SOURCE_DIR=${PROJECT_ROOT}" >>"$env_file"
	fi
}

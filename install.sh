#!/usr/bin/env sh

set -eu

REPO="${BIN_INSTALL_REPO:-aaronflorey/bin}"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"
AUTH_TOKEN="${GITHUB_AUTH_TOKEN:-${GITHUB_TOKEN:-${GH_TOKEN:-}}}"

log() {
	printf '%s\n' "$*"
}

fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

http_get() {
	url="$1"

	if command -v curl >/dev/null 2>&1; then
		if [ -n "$AUTH_TOKEN" ]; then
			curl -fsSL \
				-H "Accept: application/vnd.github+json" \
				-H "User-Agent: bin-install-script" \
				-H "Authorization: Bearer ${AUTH_TOKEN}" \
				"$url"
		else
			curl -fsSL \
				-H "Accept: application/vnd.github+json" \
				-H "User-Agent: bin-install-script" \
				"$url"
		fi
		return
	fi

	if command -v wget >/dev/null 2>&1; then
		if [ -n "$AUTH_TOKEN" ]; then
			wget -qO- \
				--header="Accept: application/vnd.github+json" \
				--header="User-Agent: bin-install-script" \
				--header="Authorization: Bearer ${AUTH_TOKEN}" \
				"$url"
		else
			wget -qO- \
				--header="Accept: application/vnd.github+json" \
				--header="User-Agent: bin-install-script" \
				"$url"
		fi
		return
	fi

	fail "either curl or wget is required"
}

download_file() {
	url="$1"
	dest="$2"

	if command -v curl >/dev/null 2>&1; then
		if [ -n "$AUTH_TOKEN" ]; then
			curl -fsSL \
				-H "User-Agent: bin-install-script" \
				-H "Authorization: Bearer ${AUTH_TOKEN}" \
				-o "$dest" \
				"$url"
		else
			curl -fsSL \
				-H "User-Agent: bin-install-script" \
				-o "$dest" \
				"$url"
		fi
		return
	fi

	if command -v wget >/dev/null 2>&1; then
		if [ -n "$AUTH_TOKEN" ]; then
			wget -qO "$dest" \
				--header="User-Agent: bin-install-script" \
				--header="Authorization: Bearer ${AUTH_TOKEN}" \
				"$url"
		else
			wget -qO "$dest" \
				--header="User-Agent: bin-install-script" \
				"$url"
		fi
		return
	fi

	fail "either curl or wget is required"
}

find_download_url() {
	json="$1"
	asset_regex="_${OS}_${ARCH}$"

	if command -v jq >/dev/null 2>&1; then
		artifacts_url="$(printf '%s\n' "$json" | jq -r \
			'.assets[]? | select(.name == "artifacts.json") | .browser_download_url' \
			| head -n 1)"
		if [ -n "$artifacts_url" ]; then
			artifacts_json="$(http_get "$artifacts_url" 2>/dev/null || true)"
			if [ -n "$artifacts_json" ]; then
				binary_name="$(printf '%s\n' "$artifacts_json" | jq -r --arg os "$OS" --arg arch "$ARCH" \
					'[.[] | select(.type == "Binary" and .goos == $os and .goarch == $arch)] | first | .name // empty' \
					| head -n 1)"
				if [ -n "$binary_name" ]; then
					download_url="$(printf '%s\n' "$json" | jq -r --arg binary_name "$binary_name" \
						'.assets[]? | select(.name == $binary_name) | .browser_download_url' \
						| head -n 1)"
					if [ -n "$download_url" ]; then
						printf '%s\n' "$download_url"
						return
					fi
				fi
			fi
		fi
	fi

	if command -v jq >/dev/null 2>&1; then
		printf '%s\n' "$json" | jq -r --arg asset_regex "$asset_regex" \
			'.assets[]?.browser_download_url | select(test($asset_regex))' \
			| head -n 1
		return
	fi

	printf '%s\n' "$json" \
		| tr ',' '\n' \
		| grep '"browser_download_url"' \
		| sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
		| grep "^https://" \
		| grep -E "_${OS}_${ARCH}$" \
		| head -n 1
}

detect_os() {
	case "$(uname -s)" in
		Darwin) printf 'darwin\n' ;;
		Linux) printf 'linux\n' ;;
		*) fail "unsupported operating system: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64|amd64) printf 'amd64\n' ;;
		aarch64|arm64) printf 'arm64\n' ;;
		*) fail "unsupported architecture: $(uname -m)" ;;
	esac
}

lookup_home_dir() {
	user_name="$1"

	if [ "${user_name}" = "$(id -un)" ]; then
		printf '%s\n' "$HOME"
		return
	fi

	home_dir="$(awk -F: -v u="$user_name" '$1 == u { print $6; exit }' /etc/passwd 2>/dev/null || true)"
	if [ -n "$home_dir" ]; then
		printf '%s\n' "$home_dir"
		return
	fi

	printf '%s\n' "$HOME"
}

OS="$(detect_os)"
ARCH="$(detect_arch)"

if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "${SUDO_USER}" != "root" ]; then
	DETECTED_USER="$SUDO_USER"
else
	DETECTED_USER="$(id -un)"
fi

DETECTED_HOME="$(lookup_home_dir "$DETECTED_USER")"

if [ "$(id -u)" -eq 0 ]; then
	INSTALL_DIR="/usr/local/bin"
else
	INSTALL_DIR="${DETECTED_HOME}/.local/bin"
fi

log "Detected OS: ${OS}"
log "Detected architecture: ${ARCH}"
log "Detected user: ${DETECTED_USER}"
log "Detected home: ${DETECTED_HOME}"
log "Install directory: ${INSTALL_DIR}"

RELEASE_JSON="$(http_get "$API_URL")"
TAG_NAME="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
[ -n "$TAG_NAME" ] || fail "failed to determine latest release tag"

DOWNLOAD_URL="$(find_download_url "$RELEASE_JSON")"
[ -n "$DOWNLOAD_URL" ] || fail "failed to find a release asset for ${OS}/${ARCH}"

BOOTSTRAP_BIN="$(mktemp /tmp/bin.XXXXXX)"
cleanup() {
	rm -f "$BOOTSTRAP_BIN"
}
trap cleanup EXIT INT TERM

log "Downloading ${TAG_NAME} from ${DOWNLOAD_URL}"
download_file "$DOWNLOAD_URL" "$BOOTSTRAP_BIN"
[ -f "$BOOTSTRAP_BIN" ] || fail "downloaded asset did not contain the bin binary"
chmod 0755 "$BOOTSTRAP_BIN"

mkdir -p "$INSTALL_DIR"
TARGET_PATH="${INSTALL_DIR}/bin"
log "Bootstrapping install via ${BOOTSTRAP_BIN}"
HOME="$DETECTED_HOME" BIN_EXE_DIR="$INSTALL_DIR" "$BOOTSTRAP_BIN" install --force "github.com/${REPO}" "$TARGET_PATH"

CONFIG_PATH="${DETECTED_HOME}/.config/bin/config.json"
if [ -f "$CONFIG_PATH" ]; then
	log "Existing config found at ${CONFIG_PATH}; skipping default_path update"
else
	log "Setting default_path to ${INSTALL_DIR}"
	HOME="$DETECTED_HOME" BIN_EXE_DIR="$INSTALL_DIR" "$TARGET_PATH" set-config default_path "$INSTALL_DIR"
fi

log "Installed bin to ${TARGET_PATH}"

case ":${PATH}:" in
	*:"${INSTALL_DIR}":*)
		;;
	*)
		log "Warning: ${INSTALL_DIR} is not currently in your PATH"
		;;
esac

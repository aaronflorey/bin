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

url_origin() {
	url_value="$1"
	case "$url_value" in
		http://*|https://*)
			:
			;;
		*)
			return 1
			;;
	esac

	scheme_part="${url_value%%://*}"
	remainder="${url_value#*://}"
	authority="${remainder%%/*}"
	[ "$authority" = "$remainder" ] || remainder="${remainder#*/}"

	case "$authority" in
		*@*)
			authority="${authority##*@}"
			;;
	esac

	host_part="$authority"
	port_part=""

	case "$authority" in
		\[*\]:*)
			host_part="${authority%:*}"
			port_part="${authority##*]:}"
			;;
		\[*\])
			host_part="$authority"
			;;
		*:*)
			host_part="${authority%%:*}"
			port_part="${authority##*:}"
			;;
	esac

	scheme_part="$(printf '%s' "$scheme_part" | tr '[:upper:]' '[:lower:]')"
	host_part="$(printf '%s' "$host_part" | tr '[:upper:]' '[:lower:]')"

	if [ -z "$port_part" ]; then
		case "$scheme_part" in
			http)
				port_part=80
				;;
			https)
				port_part=443
				;;
			*)
				return 1
				;;
		esac
	fi

	printf '%s\n' "${scheme_part}://${host_part}:${port_part}"
}

url_scheme() {
	url_value="$1"
	case "$url_value" in
		http://*|https://*)
			printf '%s\n' "${url_value%%://*}"
			return
			;;
		*)
			return 1
			;;
	esac
}

require_secure_initial_auth_url() {
	request_kind="$1"
	url="$2"

	url_scheme_value="$(url_scheme "$url")" || fail "failed to determine request scheme for ${url}"
	[ "$url_scheme_value" = "https" ] && return

	case "$request_kind" in
		metadata)
			fail "refusing authenticated metadata request over non-HTTPS URL ${url}"
			;;
		asset)
			fail "refusing authenticated asset download over non-HTTPS URL ${url}"
			;;
		*)
			fail "refusing authenticated request over non-HTTPS URL ${url}"
			;;
	esac
}

resolve_redirect_url() {
	base_url="$1"
	location_value="$2"

	case "$location_value" in
		http://*|https://*)
			printf '%s\n' "$location_value"
			return
			;;
		//*)
			printf '%s:%s\n' "${base_url%%://*}" "$location_value"
			return
			;;
	esac

	base_scheme="${base_url%%://*}"
	base_remainder="${base_url#*://}"
	base_authority="${base_remainder%%/*}"
	base_origin="${base_scheme}://${base_authority}"

	case "$location_value" in
		/*)
			printf '%s%s\n' "$base_origin" "$location_value"
			return
			;;
	esac

	base_path="/${base_remainder#*/}"
	base_path="${base_path%%\?*}"
	base_path="${base_path%%\#*}"
	if [ "$base_path" = "/$base_remainder" ]; then
		base_path="/"
	fi
	base_dir="${base_path%/*}"
	if [ -z "$base_dir" ] || [ "$base_dir" = "$base_path" ]; then
		base_dir="/"
	else
		base_dir="${base_dir}/"
	fi

	printf '%s%s%s\n' "$base_origin" "$base_dir" "$location_value"
}

read_location_header() {
	header_file="$1"
	LC_ALL=C tr -d '\r' < "$header_file" | awk 'BEGIN { IGNORECASE = 1 } /^[[:space:]]*Location:[[:space:]]*/ { sub(/^[[:space:]]*[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]+\[[^][]*\]$/, ""); location = $0 } END { print location }'
}

read_http_status_code() {
	header_file="$1"
	LC_ALL=C tr -d '\r' < "$header_file" | awk '/^[[:space:]]*HTTP\/[0-9.]+ [0-9][0-9][0-9]/ { code = $2 } END { print code }'
}

request_to_files() {
	request_url="$1"
	header_file="$2"
	body_file="$3"
	include_accept="$4"
	send_auth="$5"

	if command -v curl >/dev/null 2>&1; then
		if [ "$include_accept" = "true" ] && [ "$send_auth" = "true" ]; then
			http_code="$(curl -sS \
				-H "Accept: application/vnd.github+json" \
				-H "User-Agent: bin-install-script" \
				-H "Authorization: Bearer ${AUTH_TOKEN}" \
				-D "$header_file" \
				-o "$body_file" \
				-w '%{http_code}' \
				"$request_url")" || return 1
		elif [ "$include_accept" = "true" ]; then
			http_code="$(curl -sS \
				-H "Accept: application/vnd.github+json" \
				-H "User-Agent: bin-install-script" \
				-D "$header_file" \
				-o "$body_file" \
				-w '%{http_code}' \
				"$request_url")" || return 1
		elif [ "$send_auth" = "true" ]; then
			http_code="$(curl -sS \
				-H "User-Agent: bin-install-script" \
				-H "Authorization: Bearer ${AUTH_TOKEN}" \
				-D "$header_file" \
				-o "$body_file" \
				-w '%{http_code}' \
				"$request_url")" || return 1
		else
			http_code="$(curl -sS \
				-H "User-Agent: bin-install-script" \
				-D "$header_file" \
				-o "$body_file" \
				-w '%{http_code}' \
				"$request_url")" || return 1
		fi

		printf '%s\n' "$http_code"
		return
	fi

	if command -v wget >/dev/null 2>&1; then
		set -- -q --server-response --max-redirect=0 \
			"--header=User-Agent: bin-install-script"

		if [ "$include_accept" = "true" ]; then
			set -- "$@" "--header=Accept: application/vnd.github+json"
		fi

		if [ "$send_auth" = "true" ]; then
			set -- "$@" "--header=Authorization: Bearer ${AUTH_TOKEN}"
		fi

		if [ -n "${SSL_CERT_FILE:-}" ]; then
			set -- "$@" --ca-certificate "$SSL_CERT_FILE"
		fi

		wget "$@" -O "$body_file" "$request_url" 2>"$header_file" || true

		http_code="$(read_http_status_code "$header_file")"
		[ -n "$http_code" ] || return 1
		printf '%s\n' "$http_code"
		return
	fi

	return 1
}

metadata_redirect_target() {
	request_origin="$1"
	send_auth="$2"
	redirect_url="$3"

	redirect_origin="$(url_origin "$redirect_url")" || fail "failed to determine redirect origin for ${redirect_url}"
	[ "$redirect_origin" = "$request_origin" ] || fail "request redirect changed origin"
	REDIRECT_SEND_AUTH="$send_auth"
}

asset_redirect_target() {
	request_origin="$1"
	send_auth="$2"
	redirect_url="$3"

	redirect_scheme="$(url_scheme "$redirect_url")" || fail "failed to determine redirect scheme for ${redirect_url}"
	[ "$redirect_scheme" = "https" ] || fail "refusing insecure redirect target ${redirect_url}"

	if [ "$send_auth" = "true" ]; then
		redirect_origin="$(url_origin "$redirect_url")" || fail "failed to determine redirect origin for ${redirect_url}"
		if [ "$redirect_origin" != "$request_origin" ]; then
			REDIRECT_SEND_AUTH="false"
			return
		fi
	fi

	REDIRECT_SEND_AUTH="$send_auth"
}

fetch_to_file() {
	request_kind="$1"
	url="$2"
	dest="$3"
	include_accept="$4"
	send_auth="$5"
	max_redirects=10

	if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
		fail "either curl or wget is required"
	fi

	if [ "$send_auth" = "true" ]; then
		require_secure_initial_auth_url "$request_kind" "$url"
	fi

	request_origin="$(url_origin "$url")" || fail "failed to determine request origin for ${url}"
	current_url="$url"
	current_send_auth="$send_auth"
	redirect_count=0

	while :; do
		response_header_file="$(mktemp /tmp/bin-request-header.XXXXXX)"
		response_body_file="$(mktemp /tmp/bin-request-body.XXXXXX)"
		http_code="$(request_to_files "$current_url" "$response_header_file" "$response_body_file" "$include_accept" "$current_send_auth")" || {
			rm -f "$response_header_file" "$response_body_file"
			case "$request_kind" in
				metadata)
					fail "failed to fetch ${current_url}"
					;;
				asset)
					fail "failed to download ${current_url}"
					;;
			esac
		}

		case "$http_code" in
			301|302|303|307|308)
				location_header="$(read_location_header "$response_header_file")"
				rm -f "$response_header_file" "$response_body_file"
				[ -n "$location_header" ] || fail "redirect from ${current_url} missing Location header"
				redirect_url="$(resolve_redirect_url "$current_url" "$location_header")"
				case "$request_kind" in
					metadata)
						metadata_redirect_target "$request_origin" "$current_send_auth" "$redirect_url"
						current_send_auth="$REDIRECT_SEND_AUTH"
						;;
					asset)
						asset_redirect_target "$request_origin" "$current_send_auth" "$redirect_url"
						current_send_auth="$REDIRECT_SEND_AUTH"
						;;
				esac
				redirect_count=$((redirect_count + 1))
				[ "$redirect_count" -le "$max_redirects" ] || fail "too many redirects while fetching ${url}"
				current_url="$redirect_url"
				;;
			200|201|202|203|204|205|206)
				mv "$response_body_file" "$dest"
				rm -f "$response_header_file"
				return
				;;
			*)
				rm -f "$response_header_file" "$response_body_file"
				case "$request_kind" in
					metadata)
						fail "failed to fetch ${current_url} (HTTP ${http_code})"
						;;
					asset)
						fail "failed to download ${current_url} (HTTP ${http_code})"
						;;
				esac
				;;
		esac
	done
}

http_get() {
	url="$1"
	output_file="$(mktemp /tmp/bin-http-body.XXXXXX)"
	fetch_to_file "metadata" "$url" "$output_file" "true" "${AUTH_TOKEN:+true}"
	cat "$output_file"
	rm -f "$output_file"
}

download_file() {
	url="$1"
	dest="$2"
	fetch_to_file "asset" "$url" "$dest" "false" "${AUTH_TOKEN:+true}"
}

extract_bootstrap_binary() {
	archive_path="$1"
	asset_name="$2"
	dest="$3"
	extract_dir="$4"

	entry_normalizes_to_bin() {
		entry_name="$1"

		[ -n "$entry_name" ] || return 1

		case "$entry_name" in
			/*)
				return 1
				;;
		esac

		normalized_path=""
		remaining_path="$entry_name"
		while :; do
			case "$remaining_path" in
				*/*)
					path_component="${remaining_path%%/*}"
					remaining_path="${remaining_path#*/}"
					;;
				*)
					path_component="$remaining_path"
					remaining_path=""
					;;
			esac

			case "$path_component" in
				''|.)
					:
					;;
				..)
					if [ -z "$normalized_path" ]; then
						return 1
					fi
					case "$normalized_path" in
						*/*)
							normalized_path="${normalized_path%/*}"
							;;
						*)
							normalized_path=""
							;;
					esac
					;;
				*)
					if [ -n "$normalized_path" ]; then
						normalized_path="${normalized_path}/${path_component}"
					else
						normalized_path="$path_component"
					fi
					;;
			esac

			[ -n "$remaining_path" ] || break
		done

		[ "$normalized_path" = "bin" ]
	}

	validate_archive_bin_entries() {
		list_cmd="$1"
		archive_kind="$2"
		entries_file="${extract_dir}/archive-entries.txt"

		if ! sh -c "$list_cmd" > "$entries_file"; then
			fail "failed to inspect ${archive_kind} archive ${asset_name}"
		fi

		exact_bin_count=0
		unsafe_bin_path=""
		while IFS= read -r entry_name; do
			if [ "$entry_name" = "bin" ]; then
				exact_bin_count=$((exact_bin_count + 1))
				continue
			fi

			if entry_normalizes_to_bin "$entry_name"; then
				unsafe_bin_path="$entry_name"
				break
			fi
		done < "$entries_file"

		if [ -n "$unsafe_bin_path" ]; then
			fail "${archive_kind} archive ${asset_name} contains unsafe bin path: ${unsafe_bin_path}"
		fi

		if [ "$exact_bin_count" -eq 0 ]; then
			fail "${archive_kind} archive ${asset_name} does not contain a single root-level bin file"
		fi

		if [ "$exact_bin_count" -gt 1 ]; then
			fail "${archive_kind} archive ${asset_name} contains duplicate bin entries"
		fi
	}

	case "$asset_name" in
		*.tar.gz|*.tgz)
			validate_archive_bin_entries "tar -tf \"$archive_path\"" "tar"
			tar_entry_metadata="$(tar -tvf "$archive_path" "bin" 2>/dev/null | sed -n '1p')"
			[ -n "$tar_entry_metadata" ] || fail "tar archive ${asset_name} does not contain readable metadata for bin"
			tar_entry_type="$(printf '%s\n' "$tar_entry_metadata" | awk '{ print substr($1, 1, 1) }')"
			case "$tar_entry_type" in
				-)
					;;
				d)
					fail "tar archive ${asset_name} contains bin as a directory"
					;;
				l)
					fail "tar archive ${asset_name} contains bin as a symlink"
					;;
				h)
					fail "tar archive ${asset_name} contains bin as a hardlink"
					;;
				b|c)
					fail "tar archive ${asset_name} contains bin as a device"
					;;
				p)
					fail "tar archive ${asset_name} contains bin as a FIFO"
					;;
				*)
					fail "tar archive ${asset_name} contains bin with unsupported entry type ${tar_entry_type}"
					;;
			esac
			tar -xOf "$archive_path" "bin" > "$dest" || fail "failed to extract bin from tar archive ${asset_name}"
			return
			;;
		*.zip)
			if ! command -v unzip >/dev/null 2>&1; then
				fail "unzip is required to extract ${asset_name}"
			fi
			validate_archive_bin_entries "unzip -Z -1 \"$archive_path\"" "zip"
			zip_entry_metadata="$(unzip -Z -l "$archive_path" | awk '$NF == "bin" { print; exit }')"
			[ -n "$zip_entry_metadata" ] || fail "zip archive ${asset_name} does not contain readable metadata for bin"
			zip_entry_type="$(printf '%s\n' "$zip_entry_metadata" | awk '{ print substr($1, 1, 1) }')"
			case "$zip_entry_type" in
				-)
					;;
				d)
					fail "zip archive ${asset_name} contains bin as a directory"
					;;
				l)
					fail "zip archive ${asset_name} contains bin as a symlink"
					;;
				b|c|p|s)
					fail "zip archive ${asset_name} contains bin as a non-regular entry"
					;;
				*)
					fail "zip archive ${asset_name} contains bin with unverifiable entry type ${zip_entry_type}"
					;;
			esac
			unzip -p "$archive_path" "bin" > "$dest" || fail "failed to extract bin from zip archive ${asset_name}"
			return
			;;
		esac

	cp "$archive_path" "$dest"
}

find_download_url() {
	json="$1"
	asset_regex="_${OS}_${ARCH}(\.(tar\.gz|tgz|zip|exe))?$"

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
			'.assets[]? | select(.name | test($asset_regex)) | .browser_download_url' \
			| head -n 1
		return
	fi

	printf '%s\n' "$json" \
		| tr ',' '\n' \
		| grep '"browser_download_url"' \
		| sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
		| grep '^https://' \
		| grep -E "${asset_regex}$" \
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

detect_config_path() {
	xdg_path="${DETECTED_HOME}/.config/bin/config.json"
	legacy_path="${DETECTED_HOME}/.bin/config.json"

	if [ -f "$xdg_path" ]; then
		printf '%s\n' "$xdg_path"
		return
	fi

	if [ -f "$legacy_path" ]; then
		printf '%s\n' "$legacy_path"
		return
	fi

	printf '%s\n' "$xdg_path"
}

main() {
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

	CONFIG_PATH="$(detect_config_path)"
	CONFIG_DIR="$(dirname "$CONFIG_PATH")"
	if [ -f "$CONFIG_PATH" ]; then
		CONFIG_EXISTS="true"
	else
		CONFIG_EXISTS="false"
	fi

	mkdir -p "$CONFIG_DIR"
	if [ "$CONFIG_EXISTS" = "false" ]; then
		log "Preparing config at ${CONFIG_PATH}"
		printf '{\n  "default_path": "%s",\n  "bins": {}\n}\n' "$INSTALL_DIR" > "$CONFIG_PATH"
	fi

	if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "${SUDO_USER}" != "root" ]; then
		chown "${DETECTED_USER}:" "$CONFIG_DIR"
		[ "$CONFIG_EXISTS" = "false" ] && chown "${DETECTED_USER}:" "$CONFIG_PATH"
	fi

	RELEASE_JSON="$(http_get "$API_URL")"
	TAG_NAME="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
	[ -n "$TAG_NAME" ] || fail "failed to determine latest release tag"

	DOWNLOAD_URL="$(find_download_url "$RELEASE_JSON")"
	[ -n "$DOWNLOAD_URL" ] || fail "failed to find a release asset for ${OS}/${ARCH}"

	BOOTSTRAP_BIN="$(mktemp /tmp/bin.XXXXXX)"
	BOOTSTRAP_DOWNLOAD="$(mktemp /tmp/bin-download.XXXXXX)"
	BOOTSTRAP_DIR="$(mktemp -d /tmp/bin-install.XXXXXX)"
	BOOTSTRAP_ASSET_NAME="$(basename "$DOWNLOAD_URL")"
	cleanup() {
		rm -f "$BOOTSTRAP_BIN"
		rm -f "$BOOTSTRAP_DOWNLOAD"
		rm -rf "$BOOTSTRAP_DIR"
	}
	trap cleanup EXIT INT TERM

	log "Downloading ${TAG_NAME} from ${DOWNLOAD_URL}"
	download_file "$DOWNLOAD_URL" "$BOOTSTRAP_DOWNLOAD"
	[ -f "$BOOTSTRAP_DOWNLOAD" ] || fail "downloaded asset did not contain the bin binary"
	extract_bootstrap_binary "$BOOTSTRAP_DOWNLOAD" "$BOOTSTRAP_ASSET_NAME" "$BOOTSTRAP_BIN" "$BOOTSTRAP_DIR"
	[ -f "$BOOTSTRAP_BIN" ] || fail "downloaded asset did not contain the bin binary"
	chmod 0755 "$BOOTSTRAP_BIN"

	mkdir -p "$INSTALL_DIR"
	TARGET_PATH="${INSTALL_DIR}/bin"
	log "Bootstrapping install via ${BOOTSTRAP_BIN}"
	HOME="$DETECTED_HOME" BIN_CONFIG="$CONFIG_PATH" BIN_EXE_DIR="$INSTALL_DIR" "$BOOTSTRAP_BIN" install --force "github.com/${REPO}" "$TARGET_PATH"

	if "$TARGET_PATH" --version >/dev/null 2>&1; then
		:
	elif "$TARGET_PATH" version >/dev/null 2>&1; then
		:
	else
		fail "installed binary failed version check"
	fi

	if [ "$CONFIG_EXISTS" = "true" ]; then
		log "Existing config found at ${CONFIG_PATH}; preserving settings"
	else
		log "Initialized config with default_path=${INSTALL_DIR} at ${CONFIG_PATH}"
	fi

	log "Installed bin to ${TARGET_PATH}"

	case ":${PATH}:" in
		*:"${INSTALL_DIR}":*)
			;;
		*)
			log "Warning: ${INSTALL_DIR} is not currently in your PATH"
			;;
	esac
}

if [ "${BIN_INSTALL_LIB_ONLY:-}" = "1" ]; then
	return 0 2>/dev/null || exit 0
fi

main "$@"

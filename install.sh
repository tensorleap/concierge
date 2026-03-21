#!/bin/sh
set -eu

# Concierge installer — downloads the latest release binary from GitHub.
#
# Usage (with gh CLI authenticated — recommended for private repos):
#   curl -fsSL https://raw.githubusercontent.com/tensorleap/concierge/main/install.sh | sh
#
# With a specific version:
#   VERSION=v0.0.3 curl -fsSL https://raw.githubusercontent.com/tensorleap/concierge/main/install.sh | sh
#
# With a GitHub token (for private repos without gh CLI):
#   GITHUB_TOKEN=ghp_xxx curl -fsSL ... | sh
#
# Custom install directory:
#   INSTALL_DIR=~/.local/bin curl -fsSL ... | sh

REPO="tensorleap/concierge"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY="concierge"

main() {
    os="$(detect_os)"
    arch="$(detect_arch)"

    version="${VERSION:-}"
    if [ -z "$version" ]; then
        version="$(fetch_latest_version)"
    fi
    if [ -z "$version" ]; then
        err "could not determine latest version. Set VERSION=vX.Y.Z or install/authenticate the gh CLI"
    fi

    # Strip leading 'v' for the asset filename
    ver="${version#v}"
    asset="${BINARY}_${ver}_${os}_${arch}.tar.gz"

    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    info "Downloading ${BINARY} ${version} for ${os}/${arch}..."
    download_release_asset "$version" "$asset" "$tmpdir/$asset"
    download_release_asset "$version" "checksums.txt" "$tmpdir/checksums.txt"

    info "Verifying checksum..."
    verify_checksum "$tmpdir/$asset" "$tmpdir/checksums.txt" "$asset"

    info "Extracting..."
    tar -xzf "$tmpdir/$asset" -C "$tmpdir"

    info "Installing to ${INSTALL_DIR}/${BINARY}..."
    install_binary "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY"

    info "Done! Run 'concierge --help' to get started."
}

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       err "unsupported OS: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)             err "unsupported architecture: $(uname -m)" ;;
    esac
}

# Resolve auth header for GitHub API / download calls
auth_header() {
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        echo "Authorization: token ${GITHUB_TOKEN}"
    else
        echo ""
    fi
}

fetch_latest_version() {
    # Prefer gh CLI (handles private repo auth automatically)
    if command -v gh >/dev/null 2>&1; then
        tag="$(gh release view --repo "${REPO}" --json tagName --jq .tagName 2>/dev/null)" || true
        if [ -n "${tag:-}" ]; then
            echo "$tag"
            return
        fi
    fi

    # Fallback: GitHub API (works for public repos, or with GITHUB_TOKEN for private)
    header="$(auth_header)"
    if command -v curl >/dev/null 2>&1; then
        if [ -n "$header" ]; then
            curl -fsSL -H "$header" "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
        else
            curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
        fi
    elif command -v wget >/dev/null 2>&1; then
        if [ -n "$header" ]; then
            wget -qO- --header="$header" "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
        else
            wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
        fi
    else
        err "curl or wget is required"
    fi
}

download_release_asset() {
    tag="$1"
    asset_name="$2"
    dest="$3"

    # Prefer gh CLI for downloading (handles private repo auth)
    if command -v gh >/dev/null 2>&1; then
        if gh release download "$tag" --repo "${REPO}" --pattern "$asset_name" --dir "$(dirname "$dest")" 2>/dev/null; then
            # gh downloads to dir with original name; move if needed
            downloaded="$(dirname "$dest")/$asset_name"
            if [ "$downloaded" != "$dest" ]; then
                mv "$downloaded" "$dest"
            fi
            return
        fi
    fi

    # Fallback: direct URL download
    url="https://github.com/${REPO}/releases/download/${tag}/${asset_name}"
    header="$(auth_header)"
    if command -v curl >/dev/null 2>&1; then
        if [ -n "$header" ]; then
            curl -fsSL -H "$header" -H "Accept: application/octet-stream" -o "$dest" "$url"
        else
            curl -fsSL -o "$dest" "$url"
        fi
    elif command -v wget >/dev/null 2>&1; then
        if [ -n "$header" ]; then
            wget -qO "$dest" --header="$header" --header="Accept: application/octet-stream" "$url"
        else
            wget -qO "$dest" "$url"
        fi
    else
        err "curl or wget is required"
    fi
}

verify_checksum() {
    file="$1"
    checksums_file="$2"
    asset_name="$3"

    expected="$(grep "$asset_name" "$checksums_file" | awk '{print $1}')"
    if [ -z "$expected" ]; then
        err "checksum not found for $asset_name"
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$file" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    else
        warn "sha256sum/shasum not found — skipping checksum verification"
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        err "checksum mismatch: expected $expected, got $actual"
    fi
}

install_binary() {
    src="$1"
    dest="$2"
    destdir="$(dirname "$dest")"
    chmod +x "$src"

    # Create target directory if it doesn't exist
    if [ ! -d "$destdir" ]; then
        if mkdir -p "$destdir" 2>/dev/null; then
            :
        else
            info "Elevated permissions required to create ${destdir}"
            sudo mkdir -p "$destdir"
        fi
    fi

    if [ -w "$destdir" ]; then
        mv "$src" "$dest"
    else
        info "Elevated permissions required to install to ${destdir}"
        sudo mv "$src" "$dest"
    fi
}

info() {
    printf '\033[1;32m%s\033[0m\n' "$*"
}

warn() {
    printf '\033[1;33mwarning: %s\033[0m\n' "$*" >&2
}

err() {
    printf '\033[1;31merror: %s\033[0m\n' "$*" >&2
    exit 1
}

main

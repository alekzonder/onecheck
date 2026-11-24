#!/bin/sh
# Installer for onecheck: https://github.com/alekzonder/onecheck
#
# Downloads a release from GitHub, keeps every version in its own directory
# and points a symlink in a bin directory at the chosen one.
#
#   curl -fsSL https://raw.githubusercontent.com/alekzonder/onecheck/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --version 0.1.0
set -eu

REPO="alekzonder/onecheck"
MODULES="file http command disk systemd"
API="${ONECHECK_GITHUB_API:-https://api.github.com}"
DOWNLOAD="${ONECHECK_DOWNLOAD_URL:-https://github.com/$REPO/releases/download}"

usage() {
	cat <<EOF
Install onecheck from GitHub releases.

Usage: install.sh [options]

Options:
  --version <v>      version to install, e.g. 0.1.0 (default: the latest release)
  --root <dir>       where versions are kept, as <dir>/versions/<v>/onecheck
  --bin-dir <dir>    where the "onecheck" symlink is created
  --modules-dir <d>  where the bundled modules (file, http, command, disk, systemd) go
  --no-modules       install onecheck only, without modules
  -h, --help         show this help

Defaults:
  Linux   --root /usr/local/lib/onecheck  --bin-dir /usr/local/bin
          --modules-dir /usr/lib/onecheck/modules   (sudo is used if needed)
  macOS   --root ~/.local/share/onecheck  --bin-dir ~/.local/bin
          --modules-dir ~/.local/lib/onecheck/modules

Environment:
  ONECHECK_VERSION   same as --version
  GITHUB_TOKEN       GitHub token, raises the API rate limit

A version that is already installed is not downloaded again: the symlink is
just switched to it. Older versions are kept; delete their directories under
<root>/versions when you no longer need them.
EOF
}

say() { printf '%s\n' "$*"; }
die() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

need_value() { [ $# -ge 2 ] || die "option $1 needs a value"; }

VERSION="${ONECHECK_VERSION:-}"
ROOT=""
BIN_DIR=""
MODULES_DIR=""
WITH_MODULES=1

while [ $# -gt 0 ]; do
	case "$1" in
	--version) need_value "$@"; VERSION="$2"; shift 2 ;;
	--root) need_value "$@"; ROOT="$2"; shift 2 ;;
	--bin-dir) need_value "$@"; BIN_DIR="$2"; shift 2 ;;
	--modules-dir) need_value "$@"; MODULES_DIR="$2"; shift 2 ;;
	--no-modules) WITH_MODULES=0; shift ;;
	-h | --help) usage; exit 0 ;;
	*) die "unknown option: $1 (see --help)" ;;
	esac
done

# --- platform -------------------------------------------------------------

case "$(uname -s)" in
Linux)
	OS=linux
	: "${ROOT:=/usr/local/lib/onecheck}"
	: "${BIN_DIR:=/usr/local/bin}"
	: "${MODULES_DIR:=/usr/lib/onecheck/modules}"
	;;
Darwin)
	OS=darwin
	: "${ROOT:=$HOME/.local/share/onecheck}"
	: "${BIN_DIR:=$HOME/.local/bin}"
	: "${MODULES_DIR:=$HOME/.local/lib/onecheck/modules}"
	;;
*) die "unsupported system: $(uname -s) (onecheck is released for Linux and macOS)" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
arm64 | aarch64) ARCH=arm64 ;;
*) die "unsupported architecture: $(uname -m) (onecheck is released for amd64 and arm64)" ;;
esac

ASSET="onecheck_${OS}_${ARCH}"

# --- tools ----------------------------------------------------------------

have() { command -v "$1" >/dev/null 2>&1; }

# fetch <url> <file>
fetch() {
	if have curl; then
		if [ -n "${GITHUB_TOKEN:-}" ]; then
			curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -o "$2" "$1"
		else
			curl -fsSL -o "$2" "$1"
		fi
	elif have wget; then
		if [ -n "${GITHUB_TOKEN:-}" ]; then
			wget -q --header="Authorization: Bearer $GITHUB_TOKEN" -O "$2" "$1"
		else
			wget -q -O "$2" "$1"
		fi
	else
		die "curl or wget is required"
	fi
}

sha256() {
	if have sha256sum; then
		sha256sum "$1" | awk '{print $1}'
	elif have shasum; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "sha256sum or shasum is required to verify the download"
	fi
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# --- version --------------------------------------------------------------

if [ -z "$VERSION" ]; then
	fetch "$API/repos/$REPO/releases/latest" "$TMP/latest.json" ||
		die "cannot ask GitHub for the latest release of $REPO (rate limit? set GITHUB_TOKEN or pass --version)"
	VERSION="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$TMP/latest.json" | head -n 1)"
	[ -n "$VERSION" ] || die "GitHub did not name a latest release of $REPO; pass --version"
fi
VERSION="${VERSION#v}"
case "$VERSION" in
"" | *[!0-9A-Za-z.+-]*) die "invalid version: $VERSION" ;;
esac

VERSION_DIR="$ROOT/versions/$VERSION"
TARGET="$VERSION_DIR/onecheck"
LINK="$BIN_DIR/onecheck"

[ ! -d "$LINK" ] || [ -L "$LINK" ] || die "$LINK is a directory, refusing to replace it"

# --- privileges -----------------------------------------------------------

# writable <dir>: can we create things in dir, or in its nearest existing parent?
writable() {
	d="$1"
	while [ ! -e "$d" ]; do d="$(dirname "$d")"; done
	[ -d "$d" ] && [ -w "$d" ]
}

SUDO=""
needs_root=0
writable "$ROOT/versions" || needs_root=1
writable "$BIN_DIR" || needs_root=1
if [ "$WITH_MODULES" = 1 ]; then
	writable "$MODULES_DIR" || needs_root=1
fi
if [ "$needs_root" = 1 ] && [ "$(id -u)" != 0 ]; then
	have sudo || die "no write access to $ROOT, $BIN_DIR or $MODULES_DIR and no sudo; run as root or pass --root/--bin-dir/--modules-dir"
	say "root access is needed to write to $ROOT and $BIN_DIR, using sudo"
	SUDO="sudo"
fi

# --- download -------------------------------------------------------------

if [ -x "$TARGET" ]; then
	say "onecheck $VERSION is already in $VERSION_DIR"
else
	say "downloading onecheck $VERSION for $OS/$ARCH"
	fetch "$DOWNLOAD/v$VERSION/$ASSET" "$TMP/onecheck" ||
		die "cannot download $ASSET of version $VERSION: no such release, or no build for $OS/$ARCH"
	fetch "$DOWNLOAD/v$VERSION/checksums.txt" "$TMP/checksums.txt" ||
		die "cannot download checksums.txt of version $VERSION"

	want="$(awk -v name="$ASSET" '$2 == name || $2 == "*" name {print $1; exit}' "$TMP/checksums.txt")"
	[ -n "$want" ] || die "checksums.txt of version $VERSION has no entry for $ASSET"
	got="$(sha256 "$TMP/onecheck")"
	[ "$want" = "$got" ] || die "checksum mismatch for $ASSET: expected $want, got $got"

	chmod 0755 "$TMP/onecheck"
	"$TMP/onecheck" version >/dev/null 2>&1 || die "the downloaded onecheck $VERSION does not run on this machine"

	$SUDO mkdir -p "$VERSION_DIR"
	# copy under a temporary name first, so a visible binary is always complete
	$SUDO cp "$TMP/onecheck" "$TARGET.tmp.$$"
	$SUDO chmod 0755 "$TARGET.tmp.$$"
	$SUDO mv -f "$TARGET.tmp.$$" "$TARGET"
fi

# --- symlink --------------------------------------------------------------

$SUDO mkdir -p "$BIN_DIR"
if [ -e "$LINK" ] && [ ! -L "$LINK" ]; then
	say "replacing the existing file $LINK with a symlink"
fi
$SUDO ln -sfn "$TARGET" "$LINK"
say "installed: $LINK -> $TARGET"

# --- modules --------------------------------------------------------------

if [ "$WITH_MODULES" = 1 ]; then
	specs=""
	for m in $MODULES; do specs="$specs $m@$VERSION"; done
	say "installing modules ($MODULES) into $MODULES_DIR"
	# shellcheck disable=SC2086 # specs and SUDO are word lists on purpose
	$SUDO env GITHUB_TOKEN="${GITHUB_TOKEN:-}" "$LINK" module install "$REPO" $specs --modules-dir "$MODULES_DIR" ||
		die "onecheck $VERSION is installed, but its modules are not; retry with: $LINK module install $REPO --modules-dir $MODULES_DIR"
fi

# --- done -----------------------------------------------------------------

case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*)
	say ""
	say "note: $BIN_DIR is not in your PATH. Add this line to your shell profile (~/.zshrc, ~/.bashrc):"
	say "  export PATH=\"$BIN_DIR:\$PATH\""
	;;
esac

say ""
"$LINK" version

#!/bin/sh
# Installs Kishtin (kishtind and kishtin) on Linux or macOS from a release.
#
#   curl -fsSL https://github.com/cathalbyrnegit/kishtin/releases/latest/download/install.sh | sh
#
# KISHTIN_VERSION      a release such as v0.2.0 (default: the latest)
# KISHTIN_INSTALL_DIR  where to put the programs (default: ~/.local/bin)
# KISHTIN_RELEASE_URL  a folder holding the archives and SHA256SUMS, for a
#                      mirror or a local test (default: GitHub releases)
# KISHTIN_STARTUP      yes or no: whether Kishtin starts by itself when the
#                      computer starts (default: ask, or keep it on if it was)
#
# The archive is checked against SHA256SUMS before anything is installed,
# and must hold kishtind and kishtin and nothing else.
set -eu

repo="https://github.com/cathalbyrnegit/kishtin"
version="${KISHTIN_VERSION:-latest}"
dir="${KISHTIN_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	printf 'kishtin install: %s\n' "$1" >&2
	exit 1
}

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "this script is for Linux and macOS. On Windows, run install.ps1 in PowerShell." ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "no Kishtin build for $(uname -m): there are builds for amd64 and arm64 only." ;;
esac

if [ -n "${KISHTIN_RELEASE_URL:-}" ]; then
	base="${KISHTIN_RELEASE_URL%/}"
elif [ "$version" = latest ]; then
	base="$repo/releases/latest/download"
else
	base="$repo/releases/download/$version"
fi
archive="kishtin_${os}_${arch}.tar.gz"

download() {
	case "$1" in
	/* | file://*)
		cp "${1#file://}" "$2" 2>/dev/null || return 1
		return 0
		;;
	esac
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		fail "needs curl or wget to download Kishtin."
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		fail "needs sha256sum or shasum to check the download."
	fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Downloading Kishtin %s for %s/%s...\n' "$version" "$os" "$arch"
download "$base/$archive" "$tmp/$archive" || fail "could not download $base/$archive. Check the internet connection and the version."
download "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail "could not download $base/SHA256SUMS, so the download cannot be checked. Nothing was installed."

matches="$(awk -v f="$archive" '$2 == f || $2 == "*" f' "$tmp/SHA256SUMS")"
[ -n "$matches" ] || fail "SHA256SUMS has no entry for $archive. Nothing was installed."
[ "$(printf '%s\n' "$matches" | wc -l | tr -d ' ')" = 1 ] || fail "SHA256SUMS has more than one entry for $archive. Nothing was installed."
want="$(printf '%s\n' "$matches" | cut -d' ' -f1)"
got="$(sha256 "$tmp/$archive")"
[ "$want" = "$got" ] || fail "the download does not match SHA256SUMS (expected $want, got $got). Nothing was installed."

contents="$(tar -tzf "$tmp/$archive" | sed -e 's#^\./##' -e '/^$/d' | sort | tr '\n' ' ')"
[ "$contents" = "kishtin kishtind " ] || fail "the archive holds something other than kishtind and kishtin ($contents). Nothing was installed."

# A Kishtin the computer started is stopped while it is replaced, and
# started again below; otherwise the old version would keep running.
was_on=no
if [ -x "$dir/kishtind" ] && "$dir/kishtind" startup --json 2>/dev/null | grep -q '"on":true'; then
	was_on=yes
	"$dir/kishtind" startup stop >/dev/null 2>&1 || true
fi

mkdir -p "$tmp/x" "$dir"
tar -xzf "$tmp/$archive" -C "$tmp/x"
for p in kishtind kishtin; do
	cp "$tmp/x/$p" "$dir/$p.new"
	chmod 0755 "$dir/$p.new"
	mv -f "$dir/$p.new" "$dir/$p"
done

printf '\nInstalled %s in %s.\n' "$("$dir/kishtind" --version)" "$dir"
case ":$PATH:" in
*":$dir:"*) run="kishtind" ;;
*)
	run="$dir/kishtind"
	printf '\n%s is not on your PATH. To run kishtind by name, add this line to ~/.profile:\n  export PATH="%s:$PATH"\n' "$dir" "$dir"
	;;
esac

startup="${KISHTIN_STARTUP:-}"
if [ -z "$startup" ]; then
	if [ "$was_on" = yes ]; then
		startup=yes
	elif (: </dev/tty) 2>/dev/null; then
		printf '\nStart Kishtin by itself whenever this computer starts?\nRecommended, so nobody has to remember to start it after a restart. [Y/n] ' >/dev/tty
		answer=
		read -r answer </dev/tty || answer=n
		case "$answer" in
		[Nn]*) startup=no ;;
		*) startup=yes ;;
		esac
	else
		startup=no
	fi
fi
started=no
case "$startup" in
[Yy]*)
	printf '\n'
	if "$dir/kishtind" startup on; then
		started=yes
	else
		printf '\nKishtin could not set itself to start with this computer. Start it by hand as below.\n'
	fi
	;;
esac

if [ "$started" = yes ]; then
	printf '\nNext: open http://127.0.0.1:8080 in a browser on this computer to set it up.\n'
else
	printf '\nNext:\n  1. Start Kishtin:   %s\n  2. Open http://127.0.0.1:8080 in a browser on this computer to set it up.\n' "$run"
	printf '\nTo have Kishtin start by itself with this computer later: %s startup on\n' "$run"
fi

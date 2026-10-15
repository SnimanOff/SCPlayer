#!/bin/sh
set -eu
project_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
version=${1:-0.1.0}
case "$version" in ''|*[!0-9.]*) echo 'Version must contain numbers and dots' >&2; exit 1;; esac
if [ "$(go env GOOS)" != linux ]; then echo 'Run this script on Linux with GCC and the Fyne development libraries.' >&2; exit 1; fi
export CGO_ENABLED=1
if ! pkg-config --exists webkit2gtk-4.0 && pkg-config --exists webkit2gtk-4.1; then
    pcshim=$(mktemp -d)
    printf 'Name: webkit2gtk-4.0\nDescription: alias of webkit2gtk-4.1\nVersion: 2.0\nRequires: webkit2gtk-4.1\n' > "$pcshim/webkit2gtk-4.0.pc"
    export PKG_CONFIG_PATH="$pcshim${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
fi
go test ./...
arch=$(go env GOARCH)
stage="$project_root/dist/linux-$arch"
mkdir -p "$stage"
rm -f -- "$stage/sc-player.json"
ldflags='-s -w'
key() {
    eval "current=\${$1:-}"
    if [ -z "$current" ] && [ -f keys.env ]; then
        current=$(sed -n "s/^[[:space:]]*$1[[:space:]]*=[[:space:]]*//p" keys.env | tail -n 1 | sed 's/[[:space:]]*$//')
    fi
    printf '%s' "$current"
}
DISCORD_CLIENT_ID=$(key DISCORD_CLIENT_ID)
case "${DISCORD_CLIENT_ID:-}" in *[!0-9]*) echo 'DISCORD_CLIENT_ID must be numeric' >&2; exit 1;; esac
if [ -n "${DISCORD_CLIENT_ID:-}" ]; then ldflags="$ldflags -X sc_player/api/discord.DefaultClientID=$DISCORD_CLIENT_ID"; fi
go build -trimpath -ldflags "$ldflags" -o "$stage/sc-player" .
cp README.md sc-player.example.json packaging/linux/install.sh packaging/linux/uninstall.sh internal/icon/sc-player.png "$stage/"
chmod 755 "$stage/sc-player" "$stage/install.sh" "$stage/uninstall.sh"
if [ -n "${SC_PLAYER_CONFIG:-}" ]; then cp "$SC_PLAYER_CONFIG" "$stage/sc-player.json"; fi
tar -C "$stage" -czf "dist/SC-Player-$version-linux-$arch.tar.gz" .
if command -v dpkg-deb >/dev/null 2>&1; then
    package=$(mktemp -d)
    trap 'rm -rf -- "$package"' EXIT HUP INT TERM
    mkdir -p "$package/DEBIAN" "$package/usr/lib/sc-player" "$package/usr/bin" "$package/usr/share/applications" "$package/usr/share/doc/sc-player" "$package/usr/share/icons/hicolor/256x256/apps"
    cp "$stage/sc-player" "$package/usr/lib/sc-player/"
    cp internal/icon/sc-player.png "$package/usr/share/icons/hicolor/256x256/apps/"
    cp sc-player.example.json "$package/usr/lib/sc-player/"
    cp README.md "$package/usr/share/doc/sc-player/"
    if [ -f "$stage/sc-player.json" ]; then
        cp "$stage/sc-player.json" "$package/usr/lib/sc-player/"
        printf '%s\n' '/usr/lib/sc-player/sc-player.json' > "$package/DEBIAN/conffiles"
    fi
    ln -s ../lib/sc-player/sc-player "$package/usr/bin/sc-player"
    cp packaging/linux/sc-player.desktop "$package/usr/share/applications/"
    cat > "$package/DEBIAN/control" <<EOF
Package: sc-player
Version: $version
Section: sound
Priority: optional
Architecture: $arch
Maintainer: SC Player maintainers <sc-player@localhost>
Depends: mpv, libc6, libgl1, libegl1, libx11-6, libxcursor1, libxi6, libxinerama1, libxrandr2, libxxf86vm1, libxkbcommon0, libwayland-client0, libgtk-3-0, libwebkit2gtk-4.1-0
Recommends: gnome-keyring, dbus-user-session
Description: Compact modular SoundCloud desktop player
 Search, playlists, recommendations, mpv playback and Discord integration.
EOF
    dpkg-deb --root-owner-group --build "$package" "dist/SC-Player-$version-linux-$arch.deb"
fi
printf 'Linux packages: %s/dist\n' "$project_root"

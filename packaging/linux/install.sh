#!/bin/sh
set -eu
source_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
if [ "$(id -u)" -eq 0 ]; then default_prefix=/usr/local; else default_prefix="$HOME/.local"; fi
prefix=${SC_PLAYER_PREFIX:-$default_prefix}
case "$prefix" in /*) ;; *) echo 'SC_PLAYER_PREFIX must be absolute' >&2; exit 1;; esac
case "$prefix" in /|*\"*|*\`*|*\$*|*\\*|*%*) echo 'Unsupported installation prefix' >&2; exit 1;; esac
if [ ! -f "$source_dir/sc-player" ]; then echo 'Extract the release archive before running install.sh.' >&2; exit 1; fi
if ! command -v mpv >/dev/null 2>&1; then
    echo 'Install mpv with your package manager first (for example: sudo apt install mpv).' >&2
    exit 1
fi
app_dir="$prefix/lib/sc-player"
icon_dir="$prefix/share/icons/hicolor/256x256/apps"
mkdir -p "$app_dir" "$prefix/bin" "$prefix/share/applications" "$icon_dir"
install -m 755 "$source_dir/sc-player" "$app_dir/sc-player"
install -m 644 "$source_dir/sc-player.png" "$icon_dir/sc-player.png"
install -m 755 "$source_dir/uninstall.sh" "$app_dir/uninstall.sh"
install -m 644 "$source_dir/sc-player.example.json" "$app_dir/sc-player.example.json"
install -m 644 "$source_dir/README.md" "$app_dir/README.md"
if [ -f "$source_dir/sc-player.json" ] && [ ! -e "$app_dir/sc-player.json" ]; then
    install -m 600 "$source_dir/sc-player.json" "$app_dir/sc-player.json"
fi
ln -sfn ../lib/sc-player/sc-player "$prefix/bin/sc-player"
cat > "$prefix/share/applications/sc-player.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=SC Player
Comment=SoundCloud desktop player
Exec="$app_dir/sc-player"
Icon=$icon_dir/sc-player.png
Terminal=false
Categories=AudioVideo;Audio;Player;
EOF
if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "$prefix/share/applications" || true; fi
printf 'Installed SC Player in %s\nLaunch: %s/bin/sc-player\n' "$app_dir" "$prefix"

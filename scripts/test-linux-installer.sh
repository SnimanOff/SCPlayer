#!/bin/sh
set -eu
project_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT HUP INT TERM
mkdir -p "$work/release" "$work/tools"
cp "$project_root/packaging/linux/install.sh" "$project_root/packaging/linux/uninstall.sh" "$work/release/"
printf '#!/bin/sh\nexit 0\n' > "$work/release/sc-player"
printf '#!/bin/sh\nexit 0\n' > "$work/tools/mpv"
chmod +x "$work/tools/mpv"
printf '{}' > "$work/release/sc-player.json"
printf '{}' > "$work/release/sc-player.example.json"
printf 'Read me' > "$work/release/README.md"
printf 'png' > "$work/release/sc-player.png"
export PATH="$work/tools:$PATH"
export SC_PLAYER_PREFIX="$work/installation with spaces"
sh "$work/release/install.sh"
test -x "$SC_PLAYER_PREFIX/bin/sc-player"
test -f "$SC_PLAYER_PREFIX/share/applications/sc-player.desktop"
test -f "$SC_PLAYER_PREFIX/share/icons/hicolor/256x256/apps/sc-player.png"
grep -q "^Icon=$SC_PLAYER_PREFIX/share/icons/hicolor/256x256/apps/sc-player.png$" "$SC_PLAYER_PREFIX/share/applications/sc-player.desktop"
printf 'preserved' > "$SC_PLAYER_PREFIX/lib/sc-player/sc-player.json"
sh "$work/release/install.sh"
test "$(cat "$SC_PLAYER_PREFIX/lib/sc-player/sc-player.json")" = preserved
sh "$work/release/uninstall.sh"
test ! -e "$SC_PLAYER_PREFIX/bin/sc-player"
test ! -e "$SC_PLAYER_PREFIX/share/applications/sc-player.desktop"
test ! -e "$SC_PLAYER_PREFIX/share/icons/hicolor/256x256/apps/sc-player.png"
test -f "$SC_PLAYER_PREFIX/lib/sc-player/sc-player.json"
if SC_PLAYER_PREFIX=relative sh "$work/release/install.sh"; then echo 'Relative prefix accepted' >&2; exit 1; fi
echo 'Linux installer smoke tests passed.'

#!/bin/sh
set -eu
if [ "$(id -u)" -eq 0 ]; then default_prefix=/usr/local; else default_prefix="$HOME/.local"; fi
prefix=${SC_PLAYER_PREFIX:-$default_prefix}
case "$prefix" in /*) ;; *) echo 'SC_PLAYER_PREFIX must be absolute' >&2; exit 1;; esac
if [ "$prefix" = / ]; then echo 'Refusing root prefix' >&2; exit 1; fi
rm -f -- "$prefix/bin/sc-player" "$prefix/share/applications/sc-player.desktop" "$prefix/share/icons/hicolor/256x256/apps/sc-player.png"
for name in sc-player uninstall.sh sc-player.example.json README.md; do rm -f -- "$prefix/lib/sc-player/$name"; done
rmdir -- "$prefix/lib/sc-player" 2>/dev/null || true
printf 'Uninstalled SC Player from %s; configuration was retained.\n' "$prefix"

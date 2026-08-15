#!/usr/bin/env sh
set -eu

script=$(cat "$(dirname "$0")/install.sh")

case "$script" in
  *"/-/releases/permalink/latest/downloads"*)
    echo "install.sh must not use the invalid AtomGit permalink URL" >&2
    exit 1
    ;;
esac

printf '%s\n' "$script" | grep -F 'api.atomgit.com/api/v5/repos/' >/dev/null
printf '%s\n' "$script" | grep -F 'browser_download_url' >/dev/null
printf '%s\n' "$script" | grep -F 'current_dir="$(pwd)"' >/dev/null
printf '%s\n' "$script" | grep -F '*/certd-client)' >/dev/null
printf '%s\n' "$script" | grep -F 'tar -tzf' >/dev/null
printf '%s\n' "$script" | grep -F 'measure_download "$atomgit_url"' >/dev/null

echo "install.sh checks passed"

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
# 命令替换是否加引号不影响路径逻辑；只验证确实读取了当前目录。
printf '%s\n' "$script" | grep -E '^current_dir=([$][(]pwd[)]|"[$][(]pwd[)]")$' >/dev/null
printf '%s\n' "$script" | grep -F '*/certd-client)' >/dev/null
printf '%s\n' "$script" | grep -F 'tar -tzf' >/dev/null
printf '%s\n' "$script" | grep -F 'measure_download "$atomgit_url"' >/dev/null

echo "install.sh checks passed"

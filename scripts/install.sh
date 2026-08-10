#!/usr/bin/env sh
set -eu

repository="${CERTD_CLIENT_REPOSITORY:-certd/certd-client}"
current_dir=$(pwd)
default_dir="$current_dir/certd-client"

printf "安装目录（默认：%s）：" "$default_dir"
IFS= read -r install_dir || true
install_dir=${install_dir:-$default_dir}

case "$(uname -s)" in
  Linux) system=linux ;;
  Darwin) system=darwin ;;
  *) echo "不支持的系统：$(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) architecture=amd64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) echo "不支持的 CPU 架构：$(uname -m)" >&2; exit 1 ;;
esac

asset="certd-client-${system}-${architecture}.tar.gz"
github_url="https://github.com/${repository}/releases/latest/download/${asset}"
atomgit_url="https://atomgit.com/${repository}/-/releases/permalink/latest/downloads/${asset}"

measure_download() {
  curl --fail --location --silent --show-error --connect-timeout 5 --max-time 15 --range 0-0 --output /dev/null --write-out '%{time_total}' "$1" 2>/dev/null || true
}

github_time=$(measure_download "$github_url")
atomgit_time=$(measure_download "$atomgit_url")
primary_url=$github_url
fallback_url=$atomgit_url
primary_name=GitHub
if [ -n "$atomgit_time" ] && { [ -z "$github_time" ] || awk "BEGIN { exit !($atomgit_time < $github_time) }"; }; then
  primary_url=$atomgit_url
  fallback_url=$github_url
  primary_name=AtomGit
fi

mkdir -p "$install_dir"
archive=$(mktemp)
cleanup() { rm -f "$archive"; }
trap cleanup EXIT INT TERM

echo "从 ${primary_name} 下载 ${asset}..."
if ! curl --fail --location --show-error --output "$archive" "$primary_url"; then
  echo "${primary_name} 下载失败，尝试备用源..." >&2
  curl --fail --location --show-error --output "$archive" "$fallback_url"
fi

tar -xzf "$archive" -C "$install_dir"
chmod +x "$install_dir/certd-client"
echo "安装或更新完成：$install_dir/certd-client"
exec "$install_dir/certd-client" "$@"

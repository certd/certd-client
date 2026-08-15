#!/usr/bin/env sh
set -eu

repository="${CERTD_CLIENT_REPOSITORY:-certd/certd-client}"
current_dir=$(pwd)
case "$current_dir" in
  */certd-client) default_dir="$current_dir" ;;
  *) default_dir="$current_dir/certd-client" ;;
esac

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
atomgit_api_url="https://api.atomgit.com/api/v5/repos/${repository}/releases/latest"
atomgit_url=""
atomgit_release=$(curl --fail --location --silent --show-error "$atomgit_api_url" 2>/dev/null || true)
if [ -n "$atomgit_release" ]; then
  atomgit_url=$(printf '%s\n' "$atomgit_release" \
    | tr ',' '\n' \
    | grep -F '"browser_download_url"' \
    | grep -F "/${asset}\"" \
    | sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n 1)
  if [ -z "$atomgit_url" ]; then
    atomgit_tag=$(printf '%s\n' "$atomgit_release" \
      | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
      | head -n 1)
    if [ -n "$atomgit_tag" ]; then
      atomgit_url="https://atomgit.com/${repository}/releases/download/${atomgit_tag}/${asset}"
    fi
  fi
fi

measure_download() {
  curl --fail --location --silent --show-error --connect-timeout 5 --max-time 15 --range 0-0 --output /dev/null --write-out '%{time_total}' "$1" 2>/dev/null || true
}

github_time=$(measure_download "$github_url")
atomgit_time=""
if [ -n "$atomgit_url" ]; then
  atomgit_time=$(measure_download "$atomgit_url")
fi
primary_url=$github_url
fallback_url=$atomgit_url
primary_name=GitHub
if [ -n "$atomgit_url" ] && [ -n "$atomgit_time" ] && { [ -z "$github_time" ] || awk "BEGIN { exit !($atomgit_time < $github_time) }"; }; then
  primary_url=$atomgit_url
  fallback_url=$github_url
  primary_name=AtomGit
fi

mkdir -p "$install_dir"
archive=$(mktemp)
cleanup() { rm -f "$archive"; }
trap cleanup EXIT INT TERM

download_archive() {
  source_name=$1
  source_url=$2
  echo "从 ${source_name} 下载 ${asset}..."
  if ! curl --fail --location --show-error --output "$archive" "$source_url"; then
    return 1
  fi
  if ! tar -tzf "$archive" >/dev/null 2>&1; then
    echo "警告：${source_name} 返回的下载内容不是有效的 tar.gz 文件" >&2
    return 1
  fi
  return 0
}

if ! download_archive "$primary_name" "$primary_url"; then
  if [ -z "$fallback_url" ]; then
    echo "${primary_name} 下载失败，且没有可用的备用源" >&2
    exit 1
  fi
  echo "${primary_name} 下载失败，尝试备用源..." >&2
  fallback_name=GitHub
  if [ "$primary_name" = GitHub ]; then
    fallback_name=AtomGit
  fi
  if ! download_archive "$fallback_name" "$fallback_url"; then
    echo "GitHub 和 AtomGit 均下载失败" >&2
    exit 1
  fi
fi

tar -xzf "$archive" -C "$install_dir"
chmod +x "$install_dir/certd-client"
echo "安装或更新完成：$install_dir/certd-client"
exec "$install_dir/certd-client" "$@"

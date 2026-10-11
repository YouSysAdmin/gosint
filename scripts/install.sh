#!/bin/sh
# Gosint, Copyright (c) 2026 YouSysAdmin
#
# Downloads a gosint release for this machine from GitHub, verifies the
# checksum and installs the binary.
#
#   curl -sSfL https://raw.githubusercontent.com/yousysadmin/gosint/master/scripts/install.sh | sh
set -e

usage() {
  this=$1
  cat <<EOF
$this: download a gosint release binary from github.com/yousysadmin/gosint

Usage: $this [-b bindir] [-d] [tag]
  -b sets the installation directory, ~/.local/bin by default (or BINDIR)
  -d turns on debug logging
  [tag] is a tag from https://github.com/yousysadmin/gosint/releases
        Missing, the latest release is used.

EOF
  exit 2
}

parse_args() {
  BINDIR=${BINDIR:-$HOME/.local/bin}
  while getopts "b:dh?" arg; do
    case "$arg" in
    b) BINDIR="$OPTARG" ;;
    d) log_set_priority 10 ;;
    h | \?) usage "$0" ;;
    esac
  done
  shift $((OPTIND - 1))
  TAG=$1
}

# Every destructive step is in one function, so a curl|sh cut off by
# the network either does nothing or fails to parse before it starts.
execute() {
  tmpdir=$(mktmpdir)
  log_debug "downloading files into ${tmpdir}"
  http_download "${tmpdir}/${TARBALL}" "${TARBALL_URL}"
  http_download "${tmpdir}/${CHECKSUM_FILE_NAME}" "${CHECKSUM_FILE_URL}"
  hash_sha256_verify "${tmpdir}/${TARBALL}" "${tmpdir}/${CHECKSUM_FILE_NAME}"

  (cd "${tmpdir}" && untar "${TARBALL}")
  install -d "${BINDIR}"
  install "${tmpdir}/${BINARY}" "${BINDIR}/"
  log_info "installed ${BINDIR}/${BINARY}"
}

# What the release builds, minus windows: this script does not run
# there, the zip is on the releases page.
is_supported_platform() {
  platform=$1
  found=1
  case "$platform" in
  darwin/arm64) found=0 ;;
  linux/amd64) found=0 ;;
  linux/arm64) found=0 ;;
  esac
  return $found
}

check_platform() {
  if is_supported_platform "$PLATFORM"; then
    true
  else
    log_crit "platform $PLATFORM is not supported, see https://github.com/${PREFIX}/releases for the available builds"
    exit 1
  fi
}

tag_to_version() {
  test -z "${TAG}" && TAG="latest"
  log_info "checking GitHub for tag '${TAG}'"
  REALTAG=$(github_release "$OWNER/$REPO" "${TAG}") && true
  if test -z "$REALTAG"; then
    log_crit "unable to find '${TAG}', use 'latest' or see https://github.com/${PREFIX}/releases"
    exit 1
  fi
}

cat /dev/null <<EOF
------------------------------------------------------------------------
https://github.com/client9/shlib - portable posix shell functions
Public domain - http://unlicense.org
https://github.com/client9/shlib/blob/master/LICENSE.md
but credit (and pull requests) appreciated.
------------------------------------------------------------------------
EOF
is_command() {
  command -v "$1" >/dev/null
}
echoerr() {
  echo "$@" 1>&2
}
log_prefix() {
  echo "$0"
}
_logp=6
log_set_priority() {
  _logp="$1"
}
log_priority() {
  if test -z "$1"; then
    echo "$_logp"
    return
  fi
  [ "$1" -le "$_logp" ]
}
log_tag() {
  case $1 in
  0) echo "emerg" ;;
  1) echo "alert" ;;
  2) echo "crit" ;;
  3) echo "err" ;;
  4) echo "warning" ;;
  5) echo "notice" ;;
  6) echo "info" ;;
  7) echo "debug" ;;
  *) echo "$1" ;;
  esac
}
log_debug() {
  log_priority 7 || return 0
  echoerr "$(log_prefix)" "$(log_tag 7)" "$@"
}
log_info() {
  log_priority 6 || return 0
  echoerr "$(log_prefix)" "$(log_tag 6)" "$@"
}
log_err() {
  log_priority 3 || return 0
  echoerr "$(log_prefix)" "$(log_tag 3)" "$@"
}
log_crit() {
  log_priority 2 || return 0
  echoerr "$(log_prefix)" "$(log_tag 2)" "$@"
}
uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
  msys_nt) os="windows" ;;
  esac
  echo "$os"
}
uname_arch() {
  arch=$(uname -m)
  case $arch in
  x86_64) arch="amd64" ;;
  x86) arch="386" ;;
  i686) arch="386" ;;
  i386) arch="386" ;;
  aarch64) arch="arm64" ;;
  armv5*) arch="armv5" ;;
  armv6*) arch="armv6" ;;
  armv7*) arch="armv7" ;;
  esac
  echo ${arch}
}
untar() {
  tarball=$1
  case "${tarball}" in
  *.tar.gz | *.tgz) tar -xzf "${tarball}" ;;
  *.tar) tar -xf "${tarball}" ;;
  *.zip) unzip "${tarball}" ;;
  *)
    log_err "untar unknown archive format for ${tarball}"
    return 1
    ;;
  esac
}
mktmpdir() {
  test -z "$TMPDIR" && TMPDIR="$(mktemp -d)"
  mkdir -p "${TMPDIR}"
  echo "${TMPDIR}"
}
http_download_curl() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    code=$(curl -w '%{http_code}' -sL -o "$local_file" "$source_url")
  else
    code=$(curl -w '%{http_code}' -sL -H "$header" -o "$local_file" "$source_url")
  fi
  if [ "$code" != "200" ]; then
    log_debug "http_download_curl received HTTP status $code"
    return 1
  fi
  return 0
}
http_download_wget() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    wget -q -O "$local_file" "$source_url"
  else
    wget -q --header "$header" -O "$local_file" "$source_url"
  fi
}
http_download() {
  log_debug "http_download $2"
  if is_command curl; then
    http_download_curl "$@"
    return
  elif is_command wget; then
    http_download_wget "$@"
    return
  fi
  log_crit "http_download unable to find wget or curl"
  return 1
}
http_copy() {
  tmp=$(mktemp)
  http_download "${tmp}" "$1" "$2" || return 1
  body=$(cat "$tmp")
  rm -f "${tmp}"
  echo "$body"
}
github_release() {
  owner_repo=$1
  version=$2
  test -z "$version" && version="latest"
  if [ "$version" = "latest" ]; then
    giturl="https://github.com/${owner_repo}/releases/${version##v}"
    json=$(http_copy "$giturl" "Accept:application/json")
  else
    giturl_v="https://github.com/${owner_repo}/releases/v${version##v}"
    giturl_no_v="https://github.com/${owner_repo}/releases/${version##v}"
    json=$(http_copy "$giturl_v" "Accept:application/json" || http_copy "$giturl_no_v" "Accept:application/json")
  fi
  test -z "$json" && return 1
  version=$(echo "$json" | tr -s '\n' ' ' | sed 's/.*"tag_name":"//' | sed 's/".*//')
  test -z "$version" && return 1
  echo "$version"
}
hash_sha256() {
  TARGET=${1:-/dev/stdin}
  if is_command gsha256sum; then
    hash=$(gsha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command sha256sum; then
    hash=$(sha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command shasum; then
    hash=$(shasum -a 256 "$TARGET" 2>/dev/null) || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command openssl; then
    hash=$(openssl dgst -sha256 "$TARGET") || return 1
    echo "$hash" | sed 's/.*= //'
  else
    log_crit "hash_sha256 unable to find command to compute sha-256 hash"
    return 1
  fi
}
hash_sha256_verify() {
  TARGET=$1
  checksums=$2
  if [ -z "$checksums" ]; then
    log_err "hash_sha256_verify checksum file not specified in arg2"
    return 1
  fi

  want=$(grep "${TARGET##*/}" "${checksums}" | awk '{ print $1 }')
  if [ -z "$want" ]; then
    log_err "hash_sha256_verify unable to read checksum for '${TARGET}' in '${checksums}'"
    return 1
  fi
  got=$(hash_sha256 "$TARGET")
  if [ "$want" != "$got" ]; then
    log_err "hash_sha256_verify checksum for '$TARGET' did not verify ${want} vs $got"
    return 1
  fi
}
cat /dev/null <<EOF
------------------------------------------------------------------------
End of functions from https://github.com/client9/shlib
------------------------------------------------------------------------
EOF

PROJECT_NAME="gosint"
OWNER=yousysadmin
REPO="gosint"
BINARY=gosint
FORMAT=tar.gz
OS=$(uname_os)
ARCH=$(uname_arch)
PREFIX="$OWNER/$REPO"

log_prefix() {
  echo "$PREFIX"
}
PLATFORM="${OS}/${ARCH}"
GITHUB_DOWNLOAD=https://github.com/${OWNER}/${REPO}/releases/download

parse_args "$@"

check_platform

tag_to_version

log_info "found version: ${REALTAG} for ${TAG}/${PLATFORM}"

# The archive is named by goreleaser: gosint_v1.2.3_linux_amd64.tar.gz,
# the tag carries the v already.
NAME=${PROJECT_NAME}_${REALTAG}_${OS}_${ARCH}
TARBALL=${NAME}.${FORMAT}
TARBALL_URL=${GITHUB_DOWNLOAD}/${REALTAG}/${TARBALL}
CHECKSUM_FILE_NAME=checksums.sha256
CHECKSUM_FILE_URL=${GITHUB_DOWNLOAD}/${REALTAG}/${CHECKSUM_FILE_NAME}

execute

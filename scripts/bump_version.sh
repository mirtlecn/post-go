#!/usr/bin/env bash

set -euo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly repo_root="$(cd "${script_dir}/.." && pwd)"
readonly buildinfo_file="${repo_root}/internal/buildinfo/buildinfo.go"
readonly buildinfo_relative_path="internal/buildinfo/buildinfo.go"

usage() {
  cat <<'EOF'
Usage: ./scripts/bump_version.sh <version>

Examples:
  ./scripts/bump_version.sh 1.3.5
  ./scripts/bump_version.sh v1.3.5
EOF
}

if [[ $# -ne 1 ]]; then
  usage
  exit 1
fi

input_version="$1"
normalized_version="${input_version#v}"

if [[ ! "${normalized_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid version: ${input_version}" >&2
  exit 1
fi

tag_version="v${normalized_version}"

if [[ -n "$(git -C "${repo_root}" status --short)" ]]; then
  echo "git working tree is not clean" >&2
  exit 1
fi

if git -C "${repo_root}" rev-parse "${tag_version}" >/dev/null 2>&1; then
  echo "git tag already exists: ${tag_version}" >&2
  exit 1
fi

if [[ ! -f "${buildinfo_file}" ]]; then
  echo "build info file not found: ${buildinfo_file}" >&2
  exit 1
fi

perl -0pi -e 's/Version\s+=\s+"v\d+\.\d+\.\d+"/Version   = "'"${tag_version}"'"/' "${buildinfo_file}"
commit_message="chore: bump version to ${tag_version}"

git -C "${repo_root}" add "${buildinfo_relative_path}"
git -C "${repo_root}" commit -m "${commit_message}"
git -C "${repo_root}" tag "${tag_version}"

echo "updated Go version to ${tag_version}"
echo "created commit ${commit_message}"
echo "created git tag ${tag_version}"

#!/usr/bin/env bash
set -euo pipefail

# generate_formula.sh: Generates Homebrew formula for gh-stats from release archives and SHA256 checksums.
# Usage:
#   ./scripts/generate_formula.sh <version> <tag_name> <dist_dir> <out_file>

VERSION="${1:-0.1.0}"
VERSION_NO_V="${VERSION#v}"
TAG_NAME="${2:-v${VERSION_NO_V}}"
DIST_DIR="${3:-dist}"
OUT_FILE="${4:-Formula/gh-stats.rb}"

get_sha() {
  local filename="$1"
  local local_path="${DIST_DIR}/${filename}"

  if [ -f "$local_path" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$local_path" | cut -d' ' -f1
    elif command -v shasum >/dev/null 2>&1; then
      shasum -a 256 "$local_path" | cut -d' ' -f1
    fi
  else
    # Fallback to fetching SHA256 from GitHub Release checksums file if local archive not present
    local checksums_url="https://github.com/smford/gh-stats/releases/download/${TAG_NAME}/gh-stats_${VERSION_NO_V}_checksums.txt"
    local sha
    sha=$(curl -sSL "$checksums_url" | grep "${filename}$" | awk '{print $1}' || true)
    if [ -n "$sha" ]; then
      echo "$sha"
    else
      echo "REPLACE_WITH_SHA256"
    fi
  fi
}

SHA_DARWIN_ARM64=$(get_sha "gh-stats_${VERSION_NO_V}_darwin_arm64.tar.gz")
SHA_DARWIN_AMD64=$(get_sha "gh-stats_${VERSION_NO_V}_darwin_amd64.tar.gz")
SHA_LINUX_ARM64=$(get_sha "gh-stats_${VERSION_NO_V}_linux_arm64.tar.gz")
SHA_LINUX_AMD64=$(get_sha "gh-stats_${VERSION_NO_V}_linux_amd64.tar.gz")

mkdir -p "$(dirname "$OUT_FILE")"

cat <<EOF > "$OUT_FILE"
# typed: false
# frozen_string_literal: true

# This formula was auto-generated for gh-stats (https://github.com/smford/gh-stats).
class GhStats < Formula
  desc "SRE & developer reliability statistics for GitHub PRs and repositories"
  homepage "https://github.com/smford/gh-stats"
  version "${VERSION_NO_V}"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_amd64.tar.gz"
      sha256 "${SHA_DARWIN_AMD64}"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"
    end
  end

  def install
    bin.install "gh-stats"
  end

  test do
    assert_match "gh-stats version", shell_output("#{bin}/gh-stats -version")
  end
end
EOF

echo "Successfully generated Homebrew formula at ${OUT_FILE}"

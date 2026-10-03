# typed: false
# frozen_string_literal: true

# This formula was auto-generated for gh-stats (https://github.com/smford/gh-stats).
class GhStats < Formula
  desc "SRE & developer reliability statistics for GitHub PRs and repositories"
  homepage "https://github.com/smford/gh-stats"
  version "0.11.3"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_arm64.tar.gz"
      sha256 "800fba8b36b326abb299b8e9bf0ac810949476de95ca0356572c9efb901a2b7b"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_amd64.tar.gz"
      sha256 "3d7659e5869a56fbc1143e5c1ac3121b084d01a09990d642cb37f64d7169c176"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_arm64.tar.gz"
      sha256 "c1209760d64fdc247be41beddbf2e65adece3f856e1498b0c8ab48da3c013d33"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_amd64.tar.gz"
      sha256 "b7346f15c6ce6f0ff4844482bbad52c2b12dc7efdcfed8a446f986077a371de3"
    end
  end

  def install
    bin.install "gh-stats"
  end

  test do
    assert_match "gh-stats version", shell_output("#{bin}/gh-stats -version")
  end
end

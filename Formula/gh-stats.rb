# typed: false
# frozen_string_literal: true

# This formula was auto-generated for gh-stats (https://github.com/smford/gh-stats).
class GhStats < Formula
  desc "SRE & developer reliability statistics for GitHub PRs and repositories"
  homepage "https://github.com/smford/gh-stats"
  version "0.7.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_arm64.tar.gz"
      sha256 "57f078b7eb498d6c2103b29106c3f833cfb849efe577e58793777f74cb991f2d"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_amd64.tar.gz"
      sha256 "bd707f745a02c98e578e5b6c90baee6c359456204d020bec42b02d8c7aa23101"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_arm64.tar.gz"
      sha256 "37e53cca04fb8a7cc14c4950543adcd0593be43b82e171f7d9400e04acc5496b"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_amd64.tar.gz"
      sha256 "92235dffab91f501ed360969ce45ea9e472acb75043639a1c8755abfa473fc61"
    end
  end

  def install
    bin.install "gh-stats"
  end

  test do
    assert_match "gh-stats version", shell_output("#{bin}/gh-stats -version")
  end
end

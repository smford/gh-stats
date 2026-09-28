# typed: false
# frozen_string_literal: true

# This formula was auto-generated for gh-stats (https://github.com/smford/gh-stats).
class GhStats < Formula
  desc "SRE & developer reliability statistics for GitHub PRs and repositories"
  homepage "https://github.com/smford/gh-stats"
  version "0.9.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_arm64.tar.gz"
      sha256 "553f271e1a9ec9408abf3f99828c583228a9089dd294cac820ce96f412848775"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_amd64.tar.gz"
      sha256 "31f568fefd5a14b032ff87ad7362ad026d84242ead92a2f79497ec15e871c65a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_arm64.tar.gz"
      sha256 "2f5784440d906c5b514440889fd6ff13bb34c763db05526784f0ad7d5d669558"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_amd64.tar.gz"
      sha256 "18a7371cddd7d6daf32cf45054e8abdd87f98e2207727a6925d9bc32526ba0be"
    end
  end

  def install
    bin.install "gh-stats"
  end

  test do
    assert_match "gh-stats version", shell_output("#{bin}/gh-stats -version")
  end
end

# typed: false
# frozen_string_literal: true

# This formula was auto-generated for gh-stats (https://github.com/smford/gh-stats).
class GhStats < Formula
  desc "SRE & developer reliability statistics for GitHub PRs and repositories"
  homepage "https://github.com/smford/gh-stats"
  version "0.10.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_arm64.tar.gz"
      sha256 "803cb040bcd1ef742a161e21ee3795d4af28eff83edfaea926cf061e59494f4b"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_darwin_amd64.tar.gz"
      sha256 "0abd9efab1779275e7d3767dc79d961c16b8c1c9ad3560892784e704c58f5c41"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_arm64.tar.gz"
      sha256 "ba910e8d6dd6bd96d46155179ae18bf16766b295ea1637f5a9066f7e0cdfd8e8"
    end
    on_intel do
      url "https://github.com/smford/gh-stats/releases/download/v#{version}/gh-stats_#{version}_linux_amd64.tar.gz"
      sha256 "09ef0935d3b8c436a4c89b8179645b56c6b091e98aff37d8ec1e5b0ae51ab81b"
    end
  end

  def install
    bin.install "gh-stats"
  end

  test do
    assert_match "gh-stats version", shell_output("#{bin}/gh-stats -version")
  end
end

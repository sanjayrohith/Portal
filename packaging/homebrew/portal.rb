# typed: false
# frozen_string_literal: true

# Homebrew formula for Portal client
class Portal < Formula
  desc "Open-source localhost tunneling tool with stable custom subdomains"
  homepage "https://github.com/sanjayrohith/portal"
  version "1.0.0"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "https://github.com/sanjayrohith/portal/releases/download/v#{version}/portal_#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_ARM64_SHA256"

      def install
        bin.install "portal"
        generate_completions_from_executable(bin/"portal", "completion")
      end
    end
    on_intel do
      url "https://github.com/sanjayrohith/portal/releases/download/v#{version}/portal_#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_DARWIN_AMD64_SHA256"

      def install
        bin.install "portal"
        generate_completions_from_executable(bin/"portal", "completion")
      end
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/sanjayrohith/portal/releases/download/v#{version}/portal_#{version}_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_ARM64_SHA256"

      def install
        bin.install "portal"
        generate_completions_from_executable(bin/"portal", "completion")
      end
    end
    on_intel do
      url "https://github.com/sanjayrohith/portal/releases/download/v#{version}/portal_#{version}_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_LINUX_AMD64_SHA256"

      def install
        bin.install "portal"
        generate_completions_from_executable(bin/"portal", "completion")
      end
    end
  end

  test do
    assert_match "portal version", shell_output("#{bin}/portal --version 2>&1", 0)
  end
end

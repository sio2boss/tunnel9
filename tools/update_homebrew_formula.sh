#!/bin/bash

set -e

# Get the version from main.go
VERSION=$(./tools/version.sh)
RELEASE_DIR="./release"

# Check if release artifacts exist
if [ ! -d "$RELEASE_DIR" ]; then
    echo "Error: Release directory not found. Run 'make release' first."
    exit 1
fi

# Calculate SHA256 for each platform
APPLE_ARM64_SHA=$(shasum -a 256 "${RELEASE_DIR}/tunnel9-v${VERSION}-apple-arm64.tar.gz" | cut -d' ' -f1)
APPLE_AMD64_SHA=$(shasum -a 256 "${RELEASE_DIR}/tunnel9-v${VERSION}-apple-amd64.tar.gz" | cut -d' ' -f1)
LINUX_ARM64_SHA=$(shasum -a 256 "${RELEASE_DIR}/tunnel9-v${VERSION}-linux-arm64.tar.gz" | cut -d' ' -f1)
LINUX_AMD64_SHA=$(shasum -a 256 "${RELEASE_DIR}/tunnel9-v${VERSION}-linux-amd64.tar.gz" | cut -d' ' -f1)

echo "Updating Homebrew formula for version v${VERSION}"
echo "Apple ARM64 SHA256: $APPLE_ARM64_SHA"
echo "Apple AMD64 SHA256: $APPLE_AMD64_SHA"
echo "Linux ARM64 SHA256: $LINUX_ARM64_SHA"
echo "Linux AMD64 SHA256: $LINUX_AMD64_SHA"

# Create the updated formula
cat > homebrew/Formula/tunnel9.rb << EOF
class Tunnel9 < Formula
  desc "Terminal user interface (TUI) for managing SSH tunnels"
  homepage "https://github.com/sio2boss/tunnel9"
  version "${VERSION}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/sio2boss/tunnel9/releases/download/v#{version}/tunnel9-v#{version}-apple-arm64.tar.gz"
      sha256 "${APPLE_ARM64_SHA}"
    else
      url "https://github.com/sio2boss/tunnel9/releases/download/v#{version}/tunnel9-v#{version}-apple-amd64.tar.gz"
      sha256 "${APPLE_AMD64_SHA}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/sio2boss/tunnel9/releases/download/v#{version}/tunnel9-v#{version}-linux-arm64.tar.gz"
      sha256 "${LINUX_ARM64_SHA}"
    else
      url "https://github.com/sio2boss/tunnel9/releases/download/v#{version}/tunnel9-v#{version}-linux-amd64.tar.gz"
      sha256 "${LINUX_AMD64_SHA}"
    end
  end

  def install
    bin.install "tunnel9"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/tunnel9 --help")
  end
end
EOF

echo "1. Commit and push the changes to homebrew-tap"
echo "2. Users can then install with: brew install sio2boss/tap/tunnel9" 
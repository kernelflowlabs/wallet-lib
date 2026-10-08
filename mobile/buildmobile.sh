#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 [android|ios|all] [\"chain1 chain2 ...\"]"
  echo "  android - Build Android AAR only"
  echo "  ios     - Build iOS XCFramework only"
  echo "  all     - Build both platforms (default)"
  echo ""
  echo "  Optional chain list selects which chains to bundle; omit for all."
  echo "  Chains: evm sol trx kas apt sui substrate egld utxo"
  echo "  Example: $0 android \"evm sol trx\""
  exit 1
}

PLATFORM="${1:-all}"
CHAINS="${2:-}"

TAGS_ARG=()
if [[ -n "$CHAINS" ]]; then
  TAGS_ARG=(-tags "custom $CHAINS")
  echo "🔗 Chains: $CHAINS"
else
  echo "🔗 Chains: all"
fi

here="$(cd "$(dirname "$0")" && pwd)"
root="$here"
while [[ "$root" != "/" && ! -f "$root/go.mod" ]]; do
  root="$(dirname "$root")"
done
if [[ ! -f "$root/go.mod" ]]; then
  echo "❌ go.mod not found; run this script somewhere inside your module."
  exit 1
fi

echo "📦 Module root: $root"
cd "$root"
bash "$root/mobile/check-go-version.sh"

DIST="./dist"
mkdir -p "$DIST"

PKGS="./mobile/acc ./mobile/tx ./mobile/util ./mobile/trade"

build_android() {
  echo "=> Building ANDROID AAR..."
  # 16KB page alignment required for Android 15+ devices.
  gomobile bind -v \
    -ldflags="-s -w -extldflags=-Wl,-z,max-page-size=16384,-z,common-page-size=16384" \
    ${TAGS_ARG[@]+"${TAGS_ARG[@]}"} \
    -target=android/arm64,android/amd64 \
    -androidapi=21 \
    -o "$DIST/wallet_mobile.aar" \
    $PKGS
  echo "✅ Android Done: $DIST/wallet_mobile.aar"
}

build_ios() {
  echo "=> Building iOS XCFramework..."
  gomobile bind -v \
    -ldflags="-s -w" \
    ${TAGS_ARG[@]+"${TAGS_ARG[@]}"} \
    -target=ios/arm64\
    -o "$DIST/WalletMobile.xcframework" \
    $PKGS
  echo "✅ iOS Done: $DIST/WalletMobile.xcframework"
}

case "$PLATFORM" in
  android)
    build_android
    ;;
  ios)
    build_ios
    ;;
  all)
    build_android
    build_ios
    echo "🎉 All platforms built successfully!"
    ;;
  *)
    usage
    ;;
esac

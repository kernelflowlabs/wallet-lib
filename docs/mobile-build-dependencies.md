# Mobile build dependencies

The root `google.golang.org/genproto` requirement in `go.mod` is intentional.
Kaspad still selects the 2021 monolithic module while current Cosmos and EVM
dependencies select the split `googleapis/api` module. `gomobile bind` builds
the SDK through a generated temporary main module, where the SDK's `exclude`
directive does not apply. Without the direct modern root-module requirement,
that temporary module selects both owners of the annotations packages and
fails with an ambiguous import.

The pinned root version is new enough to exclude packages owned by the split
modules. The split modules may advance independently with gRPC updates. Do not
remove the root requirement merely because `go mod tidy` proposes doing so;
ordinary builds use module graph pruning and do not expose the gomobile
temporary-module conflict. Validate dependency changes with both the full-chain
and reduced-chain mobile builds.

## Release flavors

Mobile releases publish Full and Lite flavors for both Android and iOS. Full
contains every supported chain. Lite uses the explicit chain list in the
release workflow and currently contains EVM, Solana, and Tron. Adding a chain
to Full does not add it to Lite; the Lite list must be changed intentionally.

The release assets are `wallet_mobile_full.aar`, `wallet_mobile_lite.aar`,
`WalletMobile_full.xcframework.zip`, and
`WalletMobile_lite.xcframework.zip`. App builds should use the Lite assets.
The previous unsuffixed and `evm_sol` asset names are retired.

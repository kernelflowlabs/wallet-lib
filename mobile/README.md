# KernelFlow Wallet Mobile

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)

`gomobile`-friendly façade over [`wallet-sdk`](https://github.com/kernelflowlabs/wallet-lib)
— its offline signing core and online RPC handlers. It reshapes the rich Go APIs
into the flat, string-based, primitive types that `gomobile bind` can export to
iOS (`.xcframework`) and Android (`.aar`).

## Why it exists

`gomobile bind` only crosses the language boundary with a narrow set of types
(numbers, bool, string, `[]byte`, and structs of those). The signing SDK's rich
API — slices of structs, interfaces, generics — cannot be bound directly. This
package provides the flattened wrappers (`acc`, `tx`, `util`) that can.

## Build

```shell
sh buildmobile.sh android   # -> dist/wallet_mobile.aar
sh buildmobile.sh ios       # -> dist/WalletMobile.xcframework
sh buildmobile.sh all
```

Requires `gomobile` (and `garble` for the obfuscated iOS build).

### Selecting chains

By default every chain is bundled. Pass a chain list as the second argument to
bundle only those — excluded chains and their dependencies are dropped from the
binary:

```shell
sh buildmobile.sh android "evm sol trx"
```

Chains: `evm sol trx kas apt sui substrate egld utxo`.

## License

Apache License 2.0 — see [LICENSE](./LICENSE).

# KernelFlow Wallet SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/kernelflowlabs/wallet-sdk.svg)](https://pkg.go.dev/github.com/kernelflowlabs/wallet-sdk)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)

A multi-chain wallet SDK in Go: an **offline signing core** (account derivation,
address generation, transaction building and signing across major blockchain
families) plus an **optional online RPC layer** (fetch transaction context,
broadcast, query status). The two are split at the package level — the signing
core pulls no network dependencies, so it stays independently auditable. It
powers [KernelFlow](https://github.com/kernelflowlabs)'s white-label wallet
products, open-sourced so it can be audited and reused.

## Features

- **Offline signing core** — the `signing/*` packages pull no network code; keys
  never leave the process. Auditable as a network-free unit.
- **Optional online layer** — the `rpc/*` handlers fetch nonce / fees / UTXOs,
  broadcast, and track status when you want them; import only what you use.
- **Unified interfaces** — every chain implements the same `AccountHandler` and
  `TxBuilderHandler` contracts, so callers integrate new chains with the same code shape.
- **MPC / cold-signer friendly** — the `Build → Sign → ConcatSignature` split
  separates sig-hash generation from signature assembly, so the signing step can
  run in an isolated process (HSM, MPC node, air-gapped signer).
- **Mobile bindings** — a flat, `gomobile`-friendly façade lives in
  [`mobile/`](./mobile): `mobile/acc` and `mobile/tx` expose string / `[]byte`
  APIs, and `mobile/buildmobile.sh` builds the Android AAR / iOS XCFramework.

## Supported chains

| Family        | Generate Address | Build & Sign Tx |
| ------------- | :--------------: | :-------------: |
| EVM           | ✅ | ✅ |
| UTXO (BTC-like) | ✅ | ✅ |
| Solana        | ✅ | ✅ |
| Tron          | ✅ | ✅ |
| Aptos         | ✅ | ✅ |
| Sui           | ✅ | ✅ |
| Kaspa         | ✅ | ✅ |
| Substrate     | ✅ | ✅ |
| MultiversX    | ✅ | ✅ |
| Cosmos        | ✅ | ✅ |
| TON           | ✅ | ✅ |
| NEAR          | ✅ | ✅ |
| Stellar       | ✅ | ✅ |
| Filecoin      | ✅ | ✅ |
| Algorand      | ✅ | ✅ |

## Install

```shell
go get github.com/kernelflowlabs/wallet-sdk
```

Requires Go 1.25+ and a C toolchain (`gcc`/`clang`): some transitive
dependencies build C code via cgo.

## Quick start

```go
import (
    "github.com/kernelflowlabs/wallet-sdk/signing"
    "github.com/kernelflowlabs/wallet-sdk/signing/evm"
)

// Derive an account.
acc, _ := evm.NewAccountFromMnemonic(mnemonic, "m/44'/60'/0'/0/0")

// Build → Sign → assemble an EIP-1559 native transfer, fully offline.
b := evm.NewTxBuilder(&evm.Ingredient{
    TxType:          signing.TxTypeTransfer,
    ContractAddress: signing.MagicContactAddressForNative,
    Sender:          acc.Address(),
    Recipient:       recipient,
    Amount:          "1000000000000000",
    Nonce:           "0",
    GasLimit:        "21000",
    GasFeeCap:       "30000000000",
    GasTipCap:       "1500000000",
    IsLegacyTx:      "false",
}, "1")

_ = b.Build()
sig, _ := b.Sign(acc.PrivateKey())
rawTx, _ := b.ConcatSignature(sig, false) // broadcast via rpc/evm or your own RPC layer
```

## Architecture

| Package    | Responsibility |
| ---------- | -------------- |
| `signing`  | Shared interfaces (`AccountHandler`, `TxBuilderHandler`), constants, shared types, and the struct-tag `Validator`. |
| `signing/<family>` | Per-chain account derivation + offline transaction builder. |
| `rpc`, `rpc/<family>` | Online RPC handlers: fetch transaction context, broadcast, query status. These carry the network dependencies. |
| `crypto`   | Self-contained cryptographic layer: mnemonic generation, keystore, Shamir secret sharing. |
| `crypto/bip`, `crypto/key` | BIP-32/39/44 derivation and the ECDSA / Ed25519 / Schnorr / sr25519 signing primitives. |
| `mobile`, `mobile/acc`, `mobile/tx` | Flat, `gomobile`-friendly bindings (string / `[]byte` APIs) for iOS & Android; build with `mobile/buildmobile.sh`. |

The offline/online split is at the package level: `signing/*` pulls no network
dependencies — an auditable offline core — while `rpc/*` carries the RPC clients.
Consumers that only import `signing/*` compile no network code.

## Building and testing

```shell
go build ./...
go test ./...
```

## Security

This is cryptographic signing software. Please read [SECURITY.md](./SECURITY.md)
before use, and report vulnerabilities privately as described there.

## License

Apache License 2.0 — see [LICENSE](./LICENSE). A few files derive from
third-party code under their own licenses; see [NOTICE](./NOTICE).

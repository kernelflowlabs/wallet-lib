# Security Policy

The KernelFlow Wallet SDK produces and signs blockchain transactions and handles
private key material. We take security reports seriously.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report privately via one of:

- GitHub's [private vulnerability reporting](https://github.com/kernelflowlabs/wallet-lib/security/advisories/new)
- Email: security@kernelflow.io

Please include a description, affected versions, and a reproduction if possible.
We aim to acknowledge reports within 3 business days and to provide a remediation
timeline after triage.

## Scope

In scope:

- Incorrect signatures or malleable / invalid transactions produced by the builders.
- Private key or seed leakage (logging, memory, serialization).
- Address derivation that diverges from the relevant BIP / chain specification.
- Flaws in the cryptographic primitives or their usage.

Out of scope:

- Network / RPC behavior — the `signing/*` core performs no network I/O. The
  optional `rpc/*` layer does; functional bugs there are not in scope here.
- Issues in third-party vendored dependencies (report those upstream); tell us so
  we can pin or patch.

## Usage guidance

- Treat values returned by `PrivateKey()` / `PrivateKeyHex()` as secrets.
- Prefer the `Build → Sign → ConcatSignature` split to keep signing isolated.
- Validate all transaction inputs in your own layer in addition to the SDK's
  struct validators.

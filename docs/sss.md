# SSS

Mobile exposes three SSS functions in `util`:

- `SssSplit(mnemonic)`: three comma-separated shares, threshold two.
- `SssRecover(shares)`: recover from two or three comma-separated shares.
- `SssRecoverLegacy(shares)`: recover exactly two existing legacy shares.

Shares retain the existing fingerprint-index-hex format. There is no V2 API,
JSON share envelope or duplicate splitting implementation. Legacy recovery
uses the same proven underlying reconstruction, not a second algorithm.

The Shamir implementation is maintained under `crypto/internal/shamir` and
does not depend on the Vault server module. Its wire representation remains
the existing payload bytes followed by one nonzero coordinate byte. Fixed
Vault-generated vectors and bidirectional randomized compatibility checks
cover legacy reads and new-share interoperability before removing the old
dependency. Share coordinates and polynomial coefficients use `crypto/rand`.

Recovery validates a four-byte hex fingerprint, canonical decimal index in
0..254, unique indexes, BIP39 share payload lengths, matching payload lengths,
and unique nonzero embedded Shamir coordinates. The index is not the random
Shamir coordinate. A changed but still valid unique index cannot be detected
cryptographically by this format. Mobile recovery rejects empty comma entries.

Encryption is independent of SSS. `crypto.EncryptJWE(publicKeyPEM, plaintext)`
in `crypto/cipher.go` accepts public-key PEM bytes and arbitrary nonempty
plaintext bytes, and returns compact JWE bytes without hex encoding.
`util.SssEncryptJWE(publicKeyPEM, plaintext)` in `mobile/util/mnSss.go` accepts
strings and hex-encodes that output once. Neither knows wallet addresses,
share indexes, creation times, JSON fields or upload endpoints. No network
request is made. Existing password-based EncryptText/DecryptText are unchanged.

The mobile wrapper clears its temporary plaintext byte slice. It cannot erase
the caller's original immutable string or guarantee all memory copies are gone.

Resource limits are independent of any provider: PEM input is at most 16 KiB
and plaintext at most 1 MiB. The mobile wrapper checks string byte lengths
before copying them; crypto checks before PEM parsing or encryption. The PEM
budget accommodates supported RSA-8192 public keys. The plaintext budget bounds
mobile allocations while permitting use beyond small secret shares. This is
not permission to submit that much data to a particular provider.

The TEE business caller must separately check the final hex payload against
its 16384-character API limit and bound its JSON input before encryption. The
Account upload adapter already rejects longer ciphertext before sending it;
the mobile backup integration remains pending. Do not hardcode that provider
limit into the general-purpose encryption API.

The caller validates and serializes any application payload before encryption.
For a third-party share backup, the caller can serialize walletAddress (EIP-55),
share and createTime (Unix seconds), then encrypt that JSON. The SDK does not
construct or validate that business schema.

Encryption uses RSA-OAEP-256/A256GCM. The public key is PKIX PEM, RSA 3072–8192
bits; kid is automatically
derived from its SHA-256 fingerprint. No product-specific typ or other custom
header is added. The outer upload has
the same address/time and the encrypted hex as share. Decode hex before JWE
decryption. This is not an attestation or storage receipt.

Create one upload payload and persist it before network submission. Retries
reuse its exact ciphertext and time; encryption is intentionally randomized.
The storage provider must allow at least 16384 hex characters. A 500-character
limit cannot accommodate RSA-3072, irrespective of the simplified share format.

The mnemonic fingerprint is not a backup-generation identifier. Never mix
shares from different splitting operations. Verify recovered wallet addresses
against the expected account before accepting a recovery. The application owns
upload state and must not replace an old backup before all new copies are
confirmed. Existing 2/2 shares cannot acquire a third share: recover the
mnemonic first, then split anew. Old copied shares are not revoked by resplitting.

Changing the split result from two to three requires a coordinated mobile
consumer update and native artifact build. It must not be released to an app
that still assumes two returned shares. None of these APIs migrates stored data.

SDK builds select Go 1.26.8 via go.mod and the release workflow. The mobile
build script checks the effective toolchain and rejects Go below 1.25.13 on
the 1.25 branch or below 1.26.6 on the 1.26 branch, and prerelease versions.
These floors address GO-2026-5972; they do not replace future vulnerability
scanning. Only this module's toolchain selection changes, not the host's Go
installation. kid remains hex(SHA-256(PKIX DER)); the provider's key-selection
policy must be agreed before enabling uploads.

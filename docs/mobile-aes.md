# Mobile AES APIs

`crypto/aes.go` owns AES-GCM and legacy AES-CBC, in addition to the existing CTR functions. All use Go's standard `crypto/aes` and `crypto/cipher`; there is no custom cipher implementation.

Gomobile exposes these byte-array functions through `mobile/util`:

- `AesGcmSeal(data, key, aad)` returns a random 12-byte nonce followed by ciphertext and a 16-byte authentication tag.
- `AesGcmOpen(data, key, aad)` consumes that exact layout. Authentication failure returns no plaintext.
- `AesCbcEncrypt(data, key, iv)` and `AesCbcDecrypt(data, key, iv)` process complete blocks without padding. These exist only for compatibility with consumers' existing formats. Callers preserve their existing padding contract; new storage should use GCM.

Keys must be 16, 24, or 32 bytes. CBC IVs must be 16 bytes. Mobile methods cap plaintext at 1 MiB and GCM associated data at 64 KiB. They never log inputs or return input material in errors. Callers must not log MethodChannel arguments.

No fallback to a second cipher implementation is required in clients. An unavailable native operation must fail closed without replacing stored ciphertext or writing plaintext exports.

The nonce/ciphertext/tag layout is compatible with existing standard AES-GCM records. Consumers with a JSON envelope can split the first 12 bytes into a nonce field and the remainder into a ciphertext/tag field; decryption joins them in the same order. Keep existing key derivation, associated data and format versions when migrating an existing record.

A fixed app-reconstructible wrapping key is not a confidentiality boundary, even when its construction is obfuscated. This API does not change that limitation or any server-side PIN/possession-proof requirement.

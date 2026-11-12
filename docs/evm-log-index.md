# EVM log indices

`chainrpc.EvmLog.Index` is a `uint64` containing the node's block-wide log index. It is not the position in the transaction receipt's log slice. EVM `CheckTx`, `getTransactionLog`, and `eth_getLogs` preserve this index.

Receipt `logIndex` values are decoded from hexadecimal JSON quantities. The public log JSON uses a numeric `index` field, including when the index is zero. Missing indices default to zero; zero alone does not prove that a provider supplied an index. Non-EVM producers of `EvmLog` that do not populate this field also retain the zero value.

Existing fields are unchanged. Consumers must tolerate the additional JSON field. External Go callers using unkeyed `EvmLog` literals must update them or switch to keyed literals. Block hashes are not added by this change.

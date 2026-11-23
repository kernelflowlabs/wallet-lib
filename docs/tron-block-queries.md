# TRON API keys and block queries

Use `tron.NewHandler(rpcURL, apiKey, options ...httpc.Option)` to send `TRON-PRO-API-KEY` on every request made by that handler. An empty key omits the header. Existing callers must change `tron.NewHandler(rpcURL)` to `tron.NewHandler(rpcURL, "")`. Keep the key outside source code and pass it from application configuration.

```go
h, err := tron.NewHandler(rpcURL, apiKey,
    httpc.WithTimeout(15*time.Second),
    httpc.WithMaxResponseBytes(16<<20),
    httpc.WithRateLimit(1, 1),
)
if err != nil {
    return err
}
blockJSON, err := h.InquireChain(ctx, "getblockbynum", "61815425")
if err != nil {
    return err
}
receiptsJSON, err := h.InquireChain(ctx, "gettransactioninfobyblocknum", "61815425")
```

Both instructions accept a nonnegative decimal block height within the signed 64-bit range. They POST `{"num":61815425}` to `wallet/getblockbynum` and `wallet/gettransactioninfobyblocknum`, respectively. They return the full node JSON without projecting it into the library's limited transaction structs or converting integers through floating point. Decode `blockJSON` as a block object and `receiptsJSON` as an array of transaction receipts.

`getblockbynum` returns `block not found` when the response has no nonempty `blockID` or no non-null `block_header` object, including `{}`. A valid block with no transactions remains a successful response. An empty receipt array (`[]`) is preserved, but it alone does not establish that a block exists. These are FullNode endpoints and do not imply solidification. Deposit scanners must verify the requested height, block hash, and parent hash, and process only blocks at or below the solidified height obtained from `walletsolidity/getnowblock` before treating receipts as confirmed. Existing `getBlockByNumber` and other instructions are unchanged.

Invalid heights, malformed responses, node `Error` responses, and HTTP failures return errors. HTTP errors retain their underlying type through wrapping, so callers can use `errors.As` with `*httpc.HTTPStatusError` to inspect rate limits and other status codes.

API references: [GetBlockByNum](https://developers.tron.network/reference/wallet-getblockbynum) and [GetTransactionInfoByBlockNum](https://developers.tron.network/reference/gettransactioninfobyblocknum).

## HTTP configuration

The constructor forwards all `httpc.Option` values to `httpc.NewRequestWithOptions` and returns invalid option errors. Import `time` and `github.com/kernelflowlabs/wallet-lib/common/httpc` for the example above. The 16 MiB response limit is an example, not a provider limit; size it for the chosen batch size. Without options, the existing defaults remain: a 60-second timeout, 200 requests per second with burst 500, and no response size limit.

Options also support `httpc.WithHTTPClient` and `httpc.WithoutRateLimit`. The limiter belongs to the handler and is shared by its latest-state, solidified-state, and batch queries. Multiple handlers using the same API key do not share a quota; reuse a handler or coordinate externally. `GetHeight` now preserves underlying HTTP errors, including status and `Retry-After`, through `%w` wrapping.

The correct TronGrid header is `TRON-PRO-API-KEY`, as specified in the [official API key documentation](https://developers.tron.network/reference/select-network). The earlier `TRON-PROAPI-KEY` spelling was incorrect and is no longer sent.

## Solidified queries

- `h.GetSolidifiedHeight(ctx)` or `InquireChain(ctx, "getSolidifiedHeight", "")` requests `walletsolidity/getnowblock` and returns its decimal height. Missing height data is an error, not a zero-height success. An explicitly returned zero is valid.
- `InquireChain(ctx, "getSolidifiedTransactionInfoById", txID)` makes one request to `walletsolidity/gettransactioninfobyid`. Pass a 32-byte transaction ID as hex without `0x`. The returned JSON retains `fee` in SUN, `receipt`, `result`, logs, and all other node fields without numeric conversion. A missing receipt or mismatched ID is an error; absence can mean the transaction is not solidified yet and must not be treated as a definitive on-chain failure.

`CheckTx` retains its existing latest-state behavior and normalized result. Watchers requiring solidification and receipt fees should use the solidified receipt instruction and parse the returned receipt, rather than replacing it with `CheckTx`. No extra transaction-body request is made by the new instruction.

## Batch block queries

``InquireChain(ctx, "getblockbylimitnext", `{"startNum":100,"endNum":110}`)`` makes one request to `wallet/getblockbylimitnext`, requesting ten blocks. The start is inclusive and the end is exclusive. Parameters must satisfy `0 <= startNum < endNum <= int64 max`.

The full `{"block":[...]}` response is returned. A missing or partial range, missing block identity/header/height, out-of-order blocks, or mismatched heights is an error. A scanner must retry incomplete ranges rather than advance past them. Since this uses the FullNode endpoint, choose an end height no greater than the previously read solidified height plus one. Provider-specific batch size limits still apply.

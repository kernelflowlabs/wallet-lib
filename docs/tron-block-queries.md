# TRON API keys and block queries

Use `tron.NewHandler(rpcURL, apiKey)` to send `TRON-PROAPI-KEY` on every request made by that handler. An empty key omits the header. Existing callers must change `tron.NewHandler(rpcURL)` to `tron.NewHandler(rpcURL, "")`. Keep the key outside source code and pass it from application configuration.

```go
h, err := tron.NewHandler(rpcURL, apiKey)
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

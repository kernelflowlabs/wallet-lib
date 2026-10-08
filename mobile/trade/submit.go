package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kernelflowlabs/wallet-sdk/dex/bridgers"
)

type submitReq struct {
	Channel       string          `json:"channel"`
	Request       json.RawMessage `json:"request"`
	UserSignature string          `json:"user_signature"`
	RequestHash   string          `json:"request_hash"`
	TimeoutMs     int             `json:"timeout_ms"`
}

type submitResp struct {
	Hash  string `json:"hash,omitempty"`
	Error string `json:"error,omitempty"`
}

func SubmitIntent(reqJSON string) string {
	var req submitReq
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshal(&submitResp{Error: fmt.Sprintf("parse request: %v", err)})
	}
	switch req.Channel {
	case "bridgers":
		client := autoQuoteBridgers()
		if client == nil {
			return marshal(&submitResp{Error: "bridgers is not configured — pass bridgers.source_flag to Init"})
		}
		if req.RequestHash == "" || len(req.Request) == 0 {
			return marshal(&submitResp{Error: "request and request_hash source transaction hash required"})
		}
		var report bridgers.GenerateOrderRequest
		if err := json.Unmarshal(req.Request, &report); err != nil {
			return marshal(&submitResp{Error: fmt.Sprintf("parse Bridgers submit request: %v", err)})
		}
		report.Hash = req.RequestHash
		timeout := 15 * time.Second
		if req.TimeoutMs > 0 {
			timeout = time.Duration(req.TimeoutMs) * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		response, err := client.GenerateOrder(ctx, &report)
		if err != nil {
			return marshal(&submitResp{Error: err.Error()})
		}
		return marshal(&submitResp{Hash: response.Data.OrderID})
	case "bungee", "jupiter", "lifi", "univ2", "univ3":
		// Bungee intent mode is gone upstream (Socket V3 is tx-mode only).
		return marshal(&submitResp{Error: fmt.Sprintf("channel %q has no intent submission — read route.txData, sign, broadcast normally", req.Channel)})
	case "":
		return marshal(&submitResp{Error: "channel required"})
	}
	return marshal(&submitResp{Error: fmt.Sprintf("unknown channel: %q", req.Channel)})
}

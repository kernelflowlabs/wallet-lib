package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
)

type buildTxReq struct {
	Channel   string `json:"channel"`
	Via       string `json:"via"`
	RouteId   string `json:"route_id"`
	TimeoutMs int    `json:"timeout_ms"`
}

type buildTxResp struct {
	TxData       *dexmodel.DexTx       `json:"txData,omitempty"`
	ApprovalData *dexmodel.DexApproval `json:"approvalData,omitempty"`
	UserOp       string                `json:"userOp,omitempty"`
	Error        string                `json:"error,omitempty"`
}

func BuildTx(reqJSON string) string {
	var req buildTxReq
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshal(&buildTxResp{Error: fmt.Sprintf("parse request: %v", err)})
	}
	if req.RouteId == "" || req.Channel == "" {
		return marshal(&buildTxResp{Error: "channel and route_id required"})
	}
	timeout := 15 * time.Second
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if req.Via == "server" {
		return buildTxViaServer(ctx, req.Channel, req.RouteId)
	}
	switch req.Channel {
	case "bungee", "jupiter", "lifi", "univ2", "univ3", "bridgers":
		return marshal(&buildTxResp{Error: fmt.Sprintf("channel %q builds inline in AutoQuote — read route.txData; no separate BuildTx needed", req.Channel)})
	}
	return marshal(&buildTxResp{Error: fmt.Sprintf("unknown channel: %q", req.Channel)})
}

func buildTxViaServer(ctx context.Context, channel, routeId string) string {
	p := serverProxy()
	if p == nil {
		return marshal(&buildTxResp{Error: "via=server but server_url not configured — pass it to Init"})
	}
	q := url.Values{}
	q.Set("channel", channel)
	q.Set("routeId", routeId)
	var env struct {
		Code int          `json:"code"`
		Msg  string       `json:"msg"`
		Data *buildTxResp `json:"data"`
	}
	if err := p.Get(ctx, &env, "public/trade/buildTx", q); err != nil {
		return marshal(&buildTxResp{Error: fmt.Sprintf("proxy buildTx: %v", err)})
	}
	if env.Code != 0 {
		return marshal(&buildTxResp{Error: fmt.Sprintf("proxy code=%d msg=%s", env.Code, env.Msg)})
	}
	if env.Data == nil {
		return marshal(&buildTxResp{Error: "proxy returned nil data"})
	}
	return marshal(env.Data)
}

func marshal(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

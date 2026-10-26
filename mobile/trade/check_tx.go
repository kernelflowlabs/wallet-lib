package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/kernelflowlabs/wallet-lib/common/dexmodel"
)

type checkTxReq struct {
	Channel             string `json:"channel"`
	Via                 string `json:"via"`
	HashType            string `json:"hash_type"`
	Hash                string `json:"hash"`
	FromChain           string `json:"from_chain"`
	ToChain             string `json:"to_chain"`
	Bridge              string `json:"bridge,omitempty"`
	IncludeQuoteDetails *bool  `json:"include_quote_details,omitempty"`
	TimeoutMs           int    `json:"timeout_ms"`
}

type checkTxResp struct {
	Channel               string          `json:"channel"`
	Status                string          `json:"status"`
	ToChain               string          `json:"toChain,omitempty"`
	ToHash                string          `json:"toHash,omitempty"`
	FromHash              string          `json:"fromHash,omitempty"`
	ProviderStatus        string          `json:"providerStatus,omitempty"`
	ProviderStatusCode    string          `json:"providerStatusCode,omitempty"`
	OriginStatus          string          `json:"originStatus,omitempty"`
	DestinationStatus     string          `json:"destinationStatus,omitempty"`
	UserOp                string          `json:"userOp,omitempty"`
	RouteName             string          `json:"routeName,omitempty"`
	RouteLogoURI          string          `json:"routeLogoURI,omitempty"`
	IsDestPayloadExecuted *bool           `json:"isDestPayloadExecuted,omitempty"`
	QuoteDetails          json.RawMessage `json:"quoteDetails,omitempty"`
	Msg                   string          `json:"msg,omitempty"`
	Error                 string          `json:"error,omitempty"`
}

func CheckTx(reqJSON string) string {
	var req checkTxReq
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshal(&checkTxResp{Error: fmt.Sprintf("parse request: %v", err)})
	}
	if req.Channel == "" || req.Hash == "" || req.HashType == "" {
		return marshal(&checkTxResp{Error: "channel, hash, hash_type required"})
	}
	timeout := 8 * time.Second
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if req.Via == "server" {
		return checkTxViaServer(ctx, &req)
	}
	in := &dexmodel.DexCheckTxIn{
		Channel:             req.Channel,
		HashType:            dexmodel.DexHashType(req.HashType),
		Hash:                req.Hash,
		FromChain:           req.FromChain,
		ToChain:             req.ToChain,
		Bridge:              req.Bridge,
		IncludeQuoteDetails: req.IncludeQuoteDetails,
	}
	var out *dexmodel.DexCheckTxOut
	var err error
	switch req.Channel {
	case "jupiter":
		j := jupClient()
		if j == nil {
			return marshal(&checkTxResp{Error: "jupiter not initialized — call Init"})
		}
		out, err = j.Status(ctx, in)
	case "bungee":
		out, err = autoQuoteBungee().Status(ctx, in)
	case "lifi":
		out, err = autoQuoteLiFi().Status(ctx, in)
	case "bridgers":
		client := autoQuoteBridgers()
		if client == nil {
			return marshal(&checkTxResp{Error: "bridgers is not configured — pass bridgers.source_flag to Init"})
		}
		out, err = client.Status(ctx, in)
	case "univ2":
		chain := req.FromChain
		if chain == "" {
			chain = req.ToChain
		}
		client, release := acquireUniv2Client(chain)
		if client == nil {
			return marshal(&checkTxResp{Error: fmt.Sprintf("univ2 is not configured for chain %q", chain)})
		}
		defer release()
		out, err = client.Status(ctx, in)
	case "univ3":
		chain := req.FromChain
		if chain == "" {
			chain = req.ToChain
		}
		client, release := acquireUniv3Client(chain)
		if client == nil {
			return marshal(&checkTxResp{Error: fmt.Sprintf("univ3 is not configured for chain %q", chain)})
		}
		defer release()
		out, err = client.Status(ctx, in)
	default:
		return marshal(&checkTxResp{Error: fmt.Sprintf("unknown channel: %q", req.Channel)})
	}
	if err != nil {
		return marshal(&checkTxResp{Error: err.Error()})
	}
	if out == nil {
		return marshal(&checkTxResp{Error: "empty response"})
	}
	return marshal(&checkTxResp{
		Channel:               out.Channel,
		Status:                string(out.Status),
		ToChain:               out.ToChain,
		ToHash:                out.ToHash,
		FromHash:              out.FromHash,
		ProviderStatus:        out.ProviderStatus,
		ProviderStatusCode:    out.ProviderStatusCode,
		OriginStatus:          out.OriginStatus,
		DestinationStatus:     out.DestinationStatus,
		UserOp:                out.UserOp,
		RouteName:             out.RouteName,
		RouteLogoURI:          out.RouteLogoURI,
		IsDestPayloadExecuted: out.IsDestPayloadExecuted,
		QuoteDetails:          out.QuoteDetails,
		Msg:                   out.Msg,
	})
}

func checkTxViaServer(ctx context.Context, req *checkTxReq) string {
	p := serverProxy()
	if p == nil {
		return marshal(&checkTxResp{Error: "via=server but server_url not configured — pass it to Init"})
	}
	q := url.Values{}
	q.Set("channel", req.Channel)
	q.Set("hashType", req.HashType)
	q.Set("hash", req.Hash)
	q.Set("fromChain", req.FromChain)
	q.Set("toChain", req.ToChain)
	if req.Bridge != "" {
		q.Set("bridge", req.Bridge)
	}
	if req.IncludeQuoteDetails != nil {
		q.Set("includeQuoteDetails", strconv.FormatBool(*req.IncludeQuoteDetails))
	}
	var env struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Status  string `json:"status"`
			ToChain string `json:"toChain"`
			ToHash  string `json:"toHash"`
		} `json:"data"`
	}
	if err := p.Get(ctx, &env, "public/trade/checkTx", q); err != nil {
		return marshal(&checkTxResp{Error: fmt.Sprintf("proxy checkTx: %v", err)})
	}
	if env.Code != 0 {
		return marshal(&checkTxResp{Error: fmt.Sprintf("proxy code=%d msg=%s", env.Code, env.Msg)})
	}
	return marshal(&checkTxResp{
		Channel: req.Channel,
		Status:  env.Data.Status,
		ToChain: env.Data.ToChain,
		ToHash:  env.Data.ToHash,
	})
}

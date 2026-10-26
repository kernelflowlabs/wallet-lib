package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/dex"
	"github.com/kernelflowlabs/wallet-lib/dex/univ3"
)

type univ3PermitTypedDataReq struct {
	Chain     string              `json:"chain"`
	Permit    univ3.PermitRequest `json:"permit"`
	TimeoutMs int                 `json:"timeout_ms"`
}

type univ3PermitTypedDataResp struct {
	TypedData json.RawMessage `json:"typedData,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type univ3AttachPermitReq struct {
	Chain       string              `json:"chain"`
	Route       *dex.Route          `json:"route"`
	Permit      univ3.PermitRequest `json:"permit"`
	Signature   string              `json:"signature"`
	IfNecessary *bool               `json:"if_necessary,omitempty"`
	TimeoutMs   int                 `json:"timeout_ms"`
}

type univ3AttachPermitResp struct {
	TxData *dexmodel.DexTx `json:"txData,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func BuildUniv3PermitTypedData(reqJSON string) string {
	var req univ3PermitTypedDataReq
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshal(&univ3PermitTypedDataResp{Error: fmt.Sprintf("parse request: %v", err)})
	}
	chain := permitChain(req.Chain, req.Permit.ChainID)
	if chain == "" {
		return marshal(&univ3PermitTypedDataResp{Error: "chain or permit.chainId required"})
	}
	client, release := acquireUniv3Client(chain)
	if client == nil {
		return marshal(&univ3PermitTypedDataResp{Error: fmt.Sprintf("univ3 is not configured for chain %q", chain)})
	}
	defer release()
	ctx, cancel := permitContext(req.TimeoutMs)
	defer cancel()
	if err := client.ValidatePermit(ctx, req.Permit); err != nil {
		return marshal(&univ3PermitTypedDataResp{Error: err.Error()})
	}
	typedData, err := client.BuildPermitTypedDataJSON(req.Permit)
	if err != nil {
		return marshal(&univ3PermitTypedDataResp{Error: err.Error()})
	}
	return marshal(&univ3PermitTypedDataResp{TypedData: typedData})
}

func AttachUniv3Permit(reqJSON string) string {
	var req univ3AttachPermitReq
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return marshal(&univ3AttachPermitResp{Error: fmt.Sprintf("parse request: %v", err)})
	}
	if req.Route == nil || req.Route.Channel != univ3.Channel {
		return marshal(&univ3AttachPermitResp{Error: "an univ3 route is required"})
	}
	chain := permitChain(req.Chain, req.Permit.ChainID)
	if chain == "" {
		return marshal(&univ3AttachPermitResp{Error: "chain or permit.chainId required"})
	}
	signature, err := hexutil.Decode(strings.TrimSpace(req.Signature))
	if err != nil {
		return marshal(&univ3AttachPermitResp{Error: fmt.Sprintf("decode signature: %v", err)})
	}
	client, release := acquireUniv3Client(chain)
	if client == nil {
		return marshal(&univ3AttachPermitResp{Error: fmt.Sprintf("univ3 is not configured for chain %q", chain)})
	}
	defer release()
	ctx, cancel := permitContext(req.TimeoutMs)
	defer cancel()
	ifNecessary := true
	if req.IfNecessary != nil {
		ifNecessary = *req.IfNecessary
	}
	tx, err := client.AttachPermit(ctx, &dexmodel.DexRoute{
		TxData:       req.Route.TxData,
		ApprovalData: req.Route.ApprovalData,
		ExpiresAt:    req.Route.ExpiresAt,
	}, req.Permit, signature, ifNecessary)
	if err != nil {
		return marshal(&univ3AttachPermitResp{Error: err.Error()})
	}
	return marshal(&univ3AttachPermitResp{TxData: tx})
}

func permitChain(chain string, chainID uint64) string {
	chain = strings.TrimSpace(chain)
	if chain != "" {
		return chain
	}
	if chainID == 0 {
		return ""
	}
	return strconv.FormatUint(chainID, 10)
}

func permitContext(timeoutMs int) (context.Context, context.CancelFunc) {
	timeout := 15 * time.Second
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}
	return context.WithTimeout(context.Background(), timeout)
}

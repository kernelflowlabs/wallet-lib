package univ3

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/kernelflowlabs/wallet-lib/common/dexmodel"
)

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if c == nil || c.eth == nil {
		return nil, fmt.Errorf("univ3: client is not initialized")
	}
	if ctx == nil {
		return nil, fmt.Errorf("univ3: context is nil")
	}
	if in == nil {
		return nil, fmt.Errorf("univ3: status input is nil")
	}
	if in.HashType != "" && in.HashType != dexmodel.DexHashTypeTxHash {
		return nil, fmt.Errorf("univ3: only txHash status checks are supported")
	}
	if in.FromChain != "" && !c.matchesChain(in.FromChain) {
		return nil, fmt.Errorf("univ3: fromChain does not match %s/%d", c.config.ChainName, c.config.ChainID)
	}
	if in.ToChain != "" && !c.matchesChain(in.ToChain) {
		return nil, fmt.Errorf("univ3: toChain does not match %s/%d", c.config.ChainName, c.config.ChainID)
	}
	hashBytes, err := hexutil.Decode(in.Hash)
	if err != nil || len(hashBytes) != common.HashLength {
		return nil, fmt.Errorf("univ3: invalid transaction hash %q", in.Hash)
	}
	if err := c.ensureDeployment(ctx); err != nil {
		return nil, err
	}
	hash := common.BytesToHash(hashBytes)
	receipt, err := c.eth.TransactionReceipt(ctx, hash)
	if errors.Is(err, ethereum.NotFound) {
		return &dexmodel.DexCheckTxOut{
			Channel:  Channel,
			Status:   dexmodel.DexStatusPending,
			ToChain:  c.config.ChainName,
			ToHash:   hash.Hex(),
			FromHash: hash.Hex(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("univ3: transaction receipt: %w", err)
	}
	status := dexmodel.DexStatusFailed
	if receipt.Status == 1 {
		status = dexmodel.DexStatusSucceeded
	}
	return &dexmodel.DexCheckTxOut{
		Channel:        Channel,
		Status:         status,
		ToChain:        c.config.ChainName,
		ToHash:         hash.Hex(),
		FromHash:       hash.Hex(),
		ProviderStatus: strconv.FormatUint(receipt.Status, 10),
	}, nil
}

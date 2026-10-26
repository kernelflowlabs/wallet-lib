package trade

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/dex/univ3"
)

var univ3Clients map[string]*univ3.Client

func newUniv3Clients(configs []univ3.Config, existing map[string]*univ3.Client) (map[string]*univ3.Client, []*univ3.Client, error) {
	clients := make(map[string]*univ3.Client, len(configs)*2)
	created := make([]*univ3.Client, 0, len(configs))
	closeCreated := func() {
		closeUniv3ClientList(created)
	}
	for i, config := range configs {
		candidate, err := univ3.NewClient(config)
		if err != nil {
			closeCreated()
			return nil, nil, fmt.Errorf("univ3[%d]: %w", i, err)
		}
		normalized := candidate.Config()
		nameKey := strings.ToUpper(normalized.ChainName)
		idKey := strconv.FormatUint(normalized.ChainID, 10)
		if _, exists := clients[nameKey]; exists {
			candidate.Close()
			closeCreated()
			return nil, nil, fmt.Errorf("duplicate univ3 chain %q", normalized.ChainName)
		}
		if _, exists := clients[idKey]; exists {
			candidate.Close()
			closeCreated()
			return nil, nil, fmt.Errorf("duplicate univ3 chainId %d", normalized.ChainID)
		}
		client := candidate
		if current := existing[nameKey]; current != nil && sameUniv3Config(current.Config(), normalized) {
			candidate.Close()
			client = current
		} else {
			created = append(created, client)
		}
		clients[nameKey] = client
		clients[idKey] = client
	}
	return clients, created, nil
}

func sameUniv3Config(left, right univ3.Config) bool {
	return left.ChainName == right.ChainName &&
		left.ChainID == right.ChainID &&
		left.RPC == right.RPC &&
		left.Factory == right.Factory &&
		left.SwapRouter == right.SwapRouter &&
		left.Quoter == right.Quoter &&
		left.WrappedNative == right.WrappedNative &&
		left.DeadlineTTL == right.DeadlineTTL &&
		slices.Equal(left.FeeTiers, right.FeeTiers) &&
		slices.Equal(left.QuoteBaseTokens, right.QuoteBaseTokens)
}

func closeUniv3ClientList(clients []*univ3.Client) {
	for _, client := range clients {
		if client == nil {
			continue
		}
		client.Close()
	}
}

func closeRetiredUniv3Clients(previous, current map[string]*univ3.Client) {
	kept := make(map[*univ3.Client]struct{}, len(current))
	for _, client := range current {
		kept[client] = struct{}{}
	}
	retired := make(map[*univ3.Client]struct{}, len(previous))
	for _, client := range previous {
		if client == nil {
			continue
		}
		if _, ok := kept[client]; ok {
			continue
		}
		if _, ok := retired[client]; ok {
			continue
		}
		retired[client] = struct{}{}
		client.Close()
	}
}

func acquireUniv3Client(chain string) (*univ3.Client, func()) {
	tradeMu.RLock()
	client := univ3Clients[strings.ToUpper(strings.TrimSpace(chain))]
	if client == nil {
		tradeMu.RUnlock()
		return nil, nil
	}
	return client, tradeMu.RUnlock
}

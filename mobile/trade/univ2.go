package trade

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/dex/univ2"
)

var univ2Clients map[string]*univ2.Client

func newUniv2Clients(configs []univ2.Config, existing map[string]*univ2.Client) (map[string]*univ2.Client, []*univ2.Client, error) {
	clients := make(map[string]*univ2.Client, len(configs)*2)
	created := make([]*univ2.Client, 0, len(configs))
	closeCreated := func() {
		closeUniv2ClientList(created)
	}
	for i, config := range configs {
		candidate, err := univ2.NewClient(config)
		if err != nil {
			closeCreated()
			return nil, nil, fmt.Errorf("univ2[%d]: %w", i, err)
		}
		normalized := candidate.Config()
		nameKey := strings.ToUpper(normalized.ChainName)
		idKey := strconv.FormatUint(normalized.ChainID, 10)
		if _, exists := clients[nameKey]; exists {
			candidate.Close()
			closeCreated()
			return nil, nil, fmt.Errorf("duplicate univ2 chain %q", normalized.ChainName)
		}
		if _, exists := clients[idKey]; exists {
			candidate.Close()
			closeCreated()
			return nil, nil, fmt.Errorf("duplicate univ2 chainId %d", normalized.ChainID)
		}
		client := candidate
		if current := existing[nameKey]; current != nil && sameUniv2Config(current.Config(), normalized) {
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

func sameUniv2Config(left, right univ2.Config) bool {
	return left.ChainName == right.ChainName &&
		left.ChainID == right.ChainID &&
		left.RPC == right.RPC &&
		left.Factory == right.Factory &&
		left.Router02 == right.Router02 &&
		left.WrappedNative == right.WrappedNative &&
		left.DeadlineTTL == right.DeadlineTTL &&
		slices.Equal(left.QuoteBaseTokens, right.QuoteBaseTokens)
}

func closeUniv2ClientList(clients []*univ2.Client) {
	for _, client := range clients {
		if client == nil {
			continue
		}
		client.Close()
	}
}

func closeRetiredUniv2Clients(previous, current map[string]*univ2.Client) {
	kept := make(map[*univ2.Client]struct{}, len(current))
	for _, client := range current {
		kept[client] = struct{}{}
	}
	retired := make(map[*univ2.Client]struct{}, len(previous))
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

func acquireUniv2Client(chain string) (*univ2.Client, func()) {
	tradeMu.RLock()
	client := univ2Clients[strings.ToUpper(strings.TrimSpace(chain))]
	if client == nil {
		tradeMu.RUnlock()
		return nil, nil
	}
	return client, tradeMu.RUnlock
}

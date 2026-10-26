package trade

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kernelflowlabs/wallet-lib/dex/bridgers"
	"github.com/kernelflowlabs/wallet-lib/dex/jupiter"
	"github.com/kernelflowlabs/wallet-lib/dex/univ2"
	"github.com/kernelflowlabs/wallet-lib/dex/univ3"
)

var (
	tradeInitMu       sync.Mutex
	tradeMu           sync.RWMutex
	jup               *jupiter.Client
	jupiterAPIKey     string
	jupiterSolanaRPC  string
	jupiterFeeAccount string
	server            string
	bungeeAPIKey      string
	bungeeAffiliateID string
	bridgersConfig    *bridgers.Config
)

func Init(configJSON string) string {
	tradeInitMu.Lock()
	defer tradeInitMu.Unlock()

	var cfg struct {
		JupiterAPIKey     string           `json:"jupiter_api_key"`
		SolanaRPC         string           `json:"solana_rpc"`
		FeeAccount        string           `json:"fee_account"`
		ServerURL         string           `json:"server_url"`
		BungeeAPIKey      string           `json:"bungee_api_key"`
		BungeeAffiliateID string           `json:"bungee_affiliate_id"`
		Bridgers          *bridgers.Config `json:"bridgers,omitempty"`
		Univ2             []univ2.Config   `json:"univ2,omitempty"`
		Univ3             []univ3.Config   `json:"univ3,omitempty"`
	}
	if s := strings.TrimSpace(configJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &cfg); err != nil {
			return fmt.Sprintf("parse config: %v", err)
		}
	}
	tradeMu.RLock()
	client := jup
	reuseJupiter := client != nil &&
		jupiterAPIKey == cfg.JupiterAPIKey &&
		jupiterSolanaRPC == cfg.SolanaRPC &&
		jupiterFeeAccount == cfg.FeeAccount
	currentUniv2 := univ2Clients
	currentUniv3 := univ3Clients
	currentBridgers := brg
	currentBridgersConfig := bridgersConfig
	tradeMu.RUnlock()
	if !reuseJupiter {
		client = jupiter.NewClientWithOptions(cfg.SolanaRPC, cfg.FeeAccount, cfg.JupiterAPIKey)
	}
	univ2ByChain, createdUniv2, err := newUniv2Clients(cfg.Univ2, currentUniv2)
	if err != nil {
		return fmt.Sprintf("configure univ2: %v", err)
	}
	univ3ByChain, createdUniv3, err := newUniv3Clients(cfg.Univ3, currentUniv3)
	if err != nil {
		closeUniv2ClientList(createdUniv2)
		return fmt.Sprintf("configure univ3: %v", err)
	}
	var bridgersClient *bridgers.Client
	if cfg.Bridgers != nil {
		if currentBridgers != nil && sameBridgersConfig(currentBridgersConfig, cfg.Bridgers) {
			bridgersClient = currentBridgers
		} else {
			bridgersClient, err = bridgers.NewClient(*cfg.Bridgers)
			if err != nil {
				closeUniv2ClientList(createdUniv2)
				closeUniv3ClientList(createdUniv3)
				return fmt.Sprintf("configure bridgers: %v", err)
			}
		}
	}
	srv := strings.TrimRight(cfg.ServerURL, "/")

	tradeMu.Lock()
	previousUniv2 := univ2Clients
	previousUniv3 := univ3Clients
	jup = client
	jupiterAPIKey = cfg.JupiterAPIKey
	jupiterSolanaRPC = cfg.SolanaRPC
	jupiterFeeAccount = cfg.FeeAccount
	if server != srv {
		proxy = nil
	}
	server = srv
	if bungeeAPIKey != cfg.BungeeAPIKey || bungeeAffiliateID != cfg.BungeeAffiliateID {
		bng = nil
	}
	bungeeAPIKey = cfg.BungeeAPIKey
	bungeeAffiliateID = cfg.BungeeAffiliateID
	brg = bridgersClient
	bridgersConfig = cloneBridgersConfig(cfg.Bridgers)
	univ2Clients = univ2ByChain
	univ3Clients = univ3ByChain
	tradeMu.Unlock()
	closeRetiredUniv2Clients(previousUniv2, univ2ByChain)
	closeRetiredUniv3Clients(previousUniv3, univ3ByChain)
	return ""
}

func sameBridgersConfig(left, right *bridgers.Config) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.BaseURL == right.BaseURL &&
		left.SourceFlag == right.SourceFlag &&
		left.SourceType == right.SourceType &&
		left.FixedRateFlag == right.FixedRateFlag &&
		left.TokenCacheTTL == right.TokenCacheTTL &&
		left.RateLimit == right.RateLimit
}

func cloneBridgersConfig(config *bridgers.Config) *bridgers.Config {
	if config == nil {
		return nil
	}
	copy := *config
	copy.HTTPOptions = nil
	return &copy
}

func jupClient() *jupiter.Client {
	tradeMu.RLock()
	defer tradeMu.RUnlock()
	return jup
}

func Version() string {
	return "wallet-lib/mobile/trade 1.0.3 (dynamic univ2/univ3 + EIP-2612 permit + lifecycle-safe reconfiguration)"
}

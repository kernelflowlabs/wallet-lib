package bridgers

import (
	"strings"
	"time"

	"github.com/kernelflowlabs/wallet-sdk/common/httpc"
	"github.com/kernelflowlabs/wallet-sdk/dex"
)

const DefaultBaseURL = "https://api.bridgers.xyz"

type Config struct {
	BaseURL       string         `json:"base_url,omitempty"`
	SourceFlag    string         `json:"source_flag"`
	SourceType    string         `json:"source_type,omitempty"`
	FixedRateFlag int            `json:"fixed_rate_flag,omitempty"`
	TokenCacheTTL time.Duration  `json:"token_cache_ttl,omitempty"`
	RateLimit     dex.RateLimit  `json:"rate_limit,omitempty"`
	HTTPOptions   []httpc.Option `json:"-"`
}

func normalizeConfig(config Config) Config {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}
	config.SourceFlag = strings.TrimSpace(config.SourceFlag)
	config.SourceType = strings.TrimSpace(config.SourceType)
	if config.TokenCacheTTL <= 0 {
		config.TokenCacheTTL = 5 * time.Minute
	}
	return config
}

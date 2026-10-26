package jupiter

import "github.com/kernelflowlabs/wallet-lib/dex"

var (
	RateLimitFree = dex.RateLimit{QPS: 4, Burst: 8}
	RateLimitPro  = dex.RateLimit{QPS: 30, Burst: 60}
)

type clientConfig struct {
	rpcURL     string
	feeAccount string
	rateLimit  dex.RateLimit
	swapBase   string
	dataBase   string
	ultraBase  string
}

type ClientOption func(*clientConfig)

func WithRateLimit(r dex.RateLimit) ClientOption {
	return func(c *clientConfig) { c.rateLimit = r }
}

func WithRPC(url string) ClientOption {
	return func(c *clientConfig) { c.rpcURL = url }
}

func WithFeeAccount(addr string) ClientOption {
	return func(c *clientConfig) { c.feeAccount = addr }
}

func WithSwapBase(url string) ClientOption {
	return func(c *clientConfig) { c.swapBase = url }
}

func WithDataBase(url string) ClientOption {
	return func(c *clientConfig) { c.dataBase = url }
}

func WithUltraBase(url string) ClientOption {
	return func(c *clientConfig) { c.ultraBase = url }
}

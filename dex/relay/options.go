package relay

import "github.com/kernelflowlabs/wallet-lib/dex"

var RateLimitPublic = dex.RateLimit{QPS: 1, Burst: 2}

type clientConfig struct {
	rateLimit dex.RateLimit
	apiKey    string
	referrer  string
}

type ClientOption func(*clientConfig)

func WithRateLimit(r dex.RateLimit) ClientOption {
	return func(c *clientConfig) { c.rateLimit = r }
}

func WithApiKey(key string) ClientOption {
	return func(c *clientConfig) { c.apiKey = key }
}

func WithReferrer(referrer string) ClientOption {
	return func(c *clientConfig) { c.referrer = referrer }
}

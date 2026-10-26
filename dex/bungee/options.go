package bungee

import "github.com/kernelflowlabs/wallet-lib/dex"

var RateLimitPublic = dex.RateLimit{QPS: 1, Burst: 1}

type clientConfig struct {
	rateLimit   dex.RateLimit
	apiKey      string
	affiliateId string
}

type ClientOption func(*clientConfig)

func WithRateLimit(r dex.RateLimit) ClientOption {
	return func(c *clientConfig) { c.rateLimit = r }
}

func WithApiKey(key string) ClientOption {
	return func(c *clientConfig) { c.apiKey = key }
}

func WithAffiliateId(id string) ClientOption {
	return func(c *clientConfig) { c.affiliateId = id }
}

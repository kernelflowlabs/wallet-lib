package lifi

import "github.com/kernelflowlabs/wallet-lib/dex"

var RateLimitFree = dex.RateLimit{QPS: 1, Burst: 2}

type clientConfig struct {
	rateLimit  dex.RateLimit
	integrator string
	apiKey     string
}

type ClientOption func(*clientConfig)

func WithRateLimit(r dex.RateLimit) ClientOption {
	return func(c *clientConfig) { c.rateLimit = r }
}

func WithIntegrator(name string) ClientOption {
	return func(c *clientConfig) { c.integrator = name }
}

func WithApiKey(key string) ClientOption {
	return func(c *clientConfig) { c.apiKey = key }
}

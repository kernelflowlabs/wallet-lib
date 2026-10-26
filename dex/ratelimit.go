package dex

import "github.com/kernelflowlabs/wallet-lib/common/httpc"

type RateLimit struct {
	QPS   float64
	Burst int
}

func (r RateLimit) Apply(req *httpc.Request) {
	if req == nil || r.QPS <= 0 {
		return
	}
	burst := r.Burst
	if burst <= 0 {
		burst = 1
	}
	req.SetRateLimit(r.QPS, burst)
}

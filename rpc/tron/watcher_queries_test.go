package tron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
)

func TestWatcherQueries(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	receipt := `{"id":"` + hash + `","fee":9007199254740993,"receipt":{"result":"SUCCESS"},"log":[{"data":"abc"}]}`
	var blocks []map[string]interface{}
	for n := uint64(10); n < 20; n++ {
		blocks = append(blocks, map[string]interface{}{"blockID": fmt.Sprint(n), "block_header": map[string]interface{}{"raw_data": map[string]uint64{"number": n}}, "transactions": []string{}})
	}
	raw, _ := json.Marshal(map[string]interface{}{"block": blocks})
	for _, tc := range []struct{ instruction, params, path, body, response, want string }{
		{"getSolidifiedHeight", "", "/walletsolidity/getnowblock", "", `{"block_header":{"raw_data":{"number":19}}}`, "19"},
		{"getSolidifiedHeight", "", "/walletsolidity/getnowblock", "", `{"block_header":{"raw_data":{"number":0}}}`, "0"},
		{"getSolidifiedTransactionInfoById", hash, "/walletsolidity/gettransactioninfobyid", `{"value":"` + hash + `"}`, receipt, receipt},
		{"getblockbylimitnext", `{"startNum":10,"endNum":20}`, "/wallet/getblockbylimitnext", `{"startNum":10,"endNum":20}`, string(raw), string(raw)},
	} {
		t.Run(tc.instruction+tc.want, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != tc.path || r.Header.Get("TRON-PRO-API-KEY") != "test-key" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if tc.body != "" {
					var got json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
					if string(got) != tc.body {
						t.Errorf("body=%s want=%s", got, tc.body)
					}
				}
				w.Write([]byte(tc.response))
			}))
			defer srv.Close()
			h, err := NewHandler(srv.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := h.InquireChain(context.Background(), tc.instruction, tc.params)
			if err != nil || got != tc.want || calls != 1 {
				t.Fatalf("got=%s err=%v calls=%d", got, err, calls)
			}
		})
	}
}

func TestWatcherQueryFailures(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	for _, tc := range []struct{ instruction, params, response string }{
		{"getSolidifiedHeight", "", `{}`},
		{"getSolidifiedHeight", "", `{"block_header":{"raw_data":{}}}`},
		{"getSolidifiedHeight", "", `{"Error":"node unavailable"}`},
		{"getSolidifiedTransactionInfoById", hash, `{}`},
		{"getSolidifiedTransactionInfoById", hash, `{"id":"other"}`},
		{"getSolidifiedTransactionInfoById", hash, `[]`},
		{"getblockbylimitnext", `{"startNum":10,"endNum":20}`, `{}`},
		{"getblockbylimitnext", `{"startNum":10,"endNum":20}`, `{"block":[]}`},
		{"getblockbylimitnext", `{"startNum":10,"endNum":11}`, `{"block":[{"blockID":"x","block_header":{"raw_data":{"number":11}}}]}`},
		{"getblockbylimitnext", `{"startNum":10,"endNum":11}`, `{"block":[{}]}`},
	} {
		t.Run(tc.instruction+tc.response, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tc.response)) }))
			defer srv.Close()
			h, _ := NewHandler(srv.URL, "")
			if got, err := h.InquireChain(context.Background(), tc.instruction, tc.params); err == nil || got != "" {
				t.Fatalf("got=%s err=%v", got, err)
			}
		})
	}
	h, _ := NewHandler("http://127.0.0.1:1", "")
	for _, params := range []string{`{}`, `null`, `{"startNum":0}`, `{"startNum":-1,"endNum":1}`, `{"startNum":1,"endNum":1}`, `{"startNum":2,"endNum":1}`, `{"startNum":0,"endNum":9223372036854775808}`} {
		_, err := h.InquireChain(context.Background(), "getblockbylimitnext", params)
		if err == nil || !strings.Contains(err.Error(), "invalid params") {
			t.Fatalf("params=%s err=%v", params, err)
		}
	}
	for _, params := range []string{"", "ab", "0x" + hash, strings.Repeat("zz", 32)} {
		_, err := h.InquireChain(context.Background(), "getSolidifiedTransactionInfoById", params)
		if err == nil || !strings.Contains(err.Error(), "invalid params") {
			t.Fatalf("hash=%s err=%v", params, err)
		}
	}
}

func TestHeightAndWatcherHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "3"); w.WriteHeader(429) }))
	defer srv.Close()
	h, _ := NewHandler(srv.URL, "")
	calls := []func() (string, error){
		func() (string, error) { return h.GetHeight(context.Background()) },
		func() (string, error) { return h.GetSolidifiedHeight(context.Background()) },
		func() (string, error) {
			return h.InquireChain(context.Background(), "getSolidifiedTransactionInfoById", strings.Repeat("ab", 32))
		},
		func() (string, error) {
			return h.InquireChain(context.Background(), "getblockbylimitnext", `{"startNum":1,"endNum":11}`)
		},
	}
	for _, call := range calls {
		_, err := call()
		var status *httpc.HTTPStatusError
		if !errors.As(err, &status) || status.StatusCode != 429 || status.RetryAfterSeconds != 3 {
			t.Fatalf("status metadata lost: %v", err)
		}
	}
}

func TestHandlerHTTPOptions(t *testing.T) {
	for _, option := range []httpc.Option{nil, httpc.WithTimeout(0), httpc.WithMaxResponseBytes(0), httpc.WithRateLimit(0, 1)} {
		if h, err := NewHandler("http://localhost", "", option); err == nil || h != nil {
			t.Fatal("invalid option accepted")
		}
	}
	t.Run("size", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", 128))) }))
		defer srv.Close()
		h, err := NewHandler(srv.URL, "", httpc.WithMaxResponseBytes(64))
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.GetHeight(context.Background())
		var size *httpc.ResponseTooLargeError
		if !errors.As(err, &size) {
			t.Fatalf("size limit not preserved: %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		}))
		defer srv.Close()
		h, err := NewHandler(srv.URL, "", httpc.WithTimeout(30*time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.GetHeight(context.Background())
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("timeout not applied: %v", err)
		}
	})
	t.Run("rate", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Write([]byte(`{"block_header":{"raw_data":{"number":77}}}`))
		}))
		defer srv.Close()
		h, err := NewHandler(srv.URL, "", httpc.WithRateLimit(1, 1))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.GetHeight(context.Background()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = h.GetSolidifiedHeight(ctx)
		var limit *httpc.RateLimitError
		if !errors.As(err, &limit) || calls.Load() != 1 {
			t.Fatalf("shared rate limit not applied: %v calls=%d", err, calls.Load())
		}
	})
}

package tron

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
)

func TestHandlerAPIKey(t *testing.T) {
	for _, key := range []string{"", "test-tron-key"} {
		t.Run(key, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("TRON-PROAPI-KEY") != key {
					t.Error("unexpected API key header")
				}
				if key == "" && len(r.Header.Values("TRON-PROAPI-KEY")) != 0 {
					t.Error("empty key header must be omitted")
				}
				if r.URL.Path != "/wallet/getnowblock" {
					t.Errorf("path=%s", r.URL.Path)
				}
				w.Write([]byte(`{"block_header":{"raw_data":{"number":77}}}`))
			}))
			defer srv.Close()
			h, err := NewHandler(srv.URL, key)
			if err != nil {
				t.Fatal(err)
			}
			height, err := h.GetHeight(context.Background())
			if err != nil || height != "77" {
				t.Fatalf("height=%s err=%v", height, err)
			}
		})
	}
}

func TestInquireBlockQueries(t *testing.T) {
	for _, tc := range []struct{ instruction, response string }{
		{"getblockbynum", `{"blockID":"abc","block_header":{"raw_data":{"number":9007199254740993}},"transactions":[{"txID":"tx","raw_data":{"contract":[{"parameter":{"value":{"amount":9007199254740993}}}]},"future_field":true}]}`},
		{"getblockbynum", `{"blockID":"abc","block_header":{"raw_data":{"number":9007199254740993}},"transactions":[]}`},
		{"gettransactioninfobyblocknum", `[{"id":"tx","fee":9007199254740993,"log":[{"address":"token","topics":["topic"],"data":"value"}],"internal_transactions":[{"hash":"internal"}],"future_field":true}]`},
		{"gettransactioninfobyblocknum", `[]`},
	} {
		t.Run(tc.instruction+tc.response, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/wallet/"+tc.instruction {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("TRON-PROAPI-KEY") != "test-tron-key" {
					t.Error("missing API key")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body) != 1 || string(body["num"]) != "9007199254740993" {
					t.Errorf("body=%s", body)
				}
				w.Write([]byte(" \n" + tc.response + "\n"))
			}))
			defer srv.Close()
			h, err := NewHandler(srv.URL, "test-tron-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := h.InquireChain(context.Background(), tc.instruction, "9007199254740993")
			if err != nil || got != tc.response {
				t.Fatalf("got=%s err=%v", got, err)
			}
		})
	}
}

func TestInquireBlockQueryFailures(t *testing.T) {
	for _, instruction := range []string{"getblockbynum", "gettransactioninfobyblocknum"} {
		t.Run(instruction, func(t *testing.T) {
			calls := 0
			response := `{"Error":"node unavailable"}`
			status := http.StatusOK
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				w.Write([]byte(response))
			}))
			defer srv.Close()
			h, err := NewHandler(srv.URL, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, params := range []string{"", "-1", "+1", "0x10", "1.5", " 1", "9223372036854775808"} {
				if _, err := h.InquireChain(context.Background(), instruction, params); err == nil {
					t.Errorf("accepted params=%q", params)
				}
			}
			if calls != 0 {
				t.Fatalf("invalid params made %d requests", calls)
			}
			for _, params := range []string{"0", "9223372036854775807"} {
				if _, err := h.InquireChain(context.Background(), instruction, params); err == nil || !strings.Contains(err.Error(), "node unavailable") {
					t.Fatalf("node error=%v", err)
				}
			}
			for _, body := range []string{"", `null`, `"bad"`, `{`, `123`} {
				response = body
				if _, err := h.InquireChain(context.Background(), instruction, "1"); err == nil {
					t.Errorf("accepted response=%q", body)
				}
			}
			if instruction == "getblockbynum" {
				response = `[]`
			} else {
				response = `{}`
			}
			if _, err := h.InquireChain(context.Background(), instruction, "1"); err == nil {
				t.Error("accepted wrong response shape")
			}
			status = http.StatusTooManyRequests
			_, err = h.InquireChain(context.Background(), instruction, "1")
			var statusErr *httpc.HTTPStatusError
			if !errors.As(err, &statusErr) || statusErr.StatusCode != status {
				t.Fatalf("HTTP status error lost: %v", err)
			}
		})
	}
}

func TestGetBlockByNumNotFound(t *testing.T) {
	for _, response := range []string{
		`{}`,
		`{"block_header":{"raw_data":{"number":99999999}}}`,
		`{"blockID":"","block_header":{}}`,
		`{"blockID":null,"block_header":{}}`,
		`{"blockID":"abc"}`,
		`{"blockID":"abc","block_header":null}`,
	} {
		t.Run(response, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(response))
			}))
			defer srv.Close()
			h, err := NewHandler(srv.URL, "")
			if err != nil {
				t.Fatal(err)
			}
			got, err := h.InquireChain(context.Background(), "getblockbynum", "99999999")
			if got != "" || err == nil || err.Error() != "block not found" {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}

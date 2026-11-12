package evm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
)

func TestReceiptLogIndices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var result json.RawMessage
		switch request.Method {
		case "eth_getTransactionByHash":
			result = json.RawMessage(`{"hash":"0x123","blockNumber":"0x10"}`)
		case "eth_getTransactionReceipt":
			result = json.RawMessage(`{"blockNumber":"0x10","status":"0x1","logs":[{"address":"0xabc","topics":["0x01","0x02"],"data":"0xff","logIndex":"0x0"},{"address":"0xabc","topics":["0x01","0x02"],"data":"0xff","logIndex":"0x10000002a"}]}`)
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": request.ID, "result": result}); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	h, err := NewHandler(srv.URL, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer h.rpc.Close()
	got, err := h.CheckTx(context.Background(), "0x123")
	if err != nil {
		t.Fatal(err)
	}
	check := func(logs []chainrpc.EvmLog) {
		t.Helper()
		if len(logs) != 2 {
			t.Fatalf("logs=%+v", logs)
		}
		for i, index := range []uint64{0, 0x10000002a} {
			log := logs[i]
			if log.Index != index || log.Address != "0xabc" || log.Topics != "0x01,0x02" || log.Data != "0xff" {
				t.Errorf("log[%d]=%+v, expected index %d", i, log, index)
			}
			var fields map[string]interface{}
			if err := json.Unmarshal([]byte(log.String()), &fields); err != nil {
				t.Fatal(err)
			}
			if fields["index"] != float64(index) {
				t.Errorf("serialized index=%v", fields["index"])
			}
		}
	}
	check(got.Logs)
	raw, err := h.InquireChain(context.Background(), "getTransactionLog", "0x123")
	if err != nil {
		t.Fatal(err)
	}
	var logs []chainrpc.EvmLog
	for _, part := range strings.Split(raw, "_") {
		var log chainrpc.EvmLog
		if err := json.Unmarshal([]byte(part), &log); err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
	}
	check(logs)
}

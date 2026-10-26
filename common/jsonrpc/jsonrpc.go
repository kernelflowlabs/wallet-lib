package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
)

type JSONRPCError struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	if e == nil {
		return "JSON-RPC error"
	}
	return fmt.Sprintf("JSON-RPC error code=%d: %s", e.Code, e.Message)
}

type JSONRPCResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCClient struct {
	transport *httpc.Request
	nextID    atomic.Uint64
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCWireResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type jsonRPCWireError struct {
	Code    *int64          `json:"code"`
	Message *string         `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func NewJSONRPCClient(options ...httpc.Option) (*JSONRPCClient, error) {
	transport, err := httpc.NewRequestWithOptions("", map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
	}, options...)
	if err != nil {
		return nil, fmt.Errorf("create JSON-RPC transport: %w", err)
	}
	return &JSONRPCClient{transport: transport}, nil
}

func (c *JSONRPCClient) Call(
	ctx context.Context,
	endpoint string,
	method string,
	params json.RawMessage,
) (JSONRPCResponse, error) {
	if c == nil || c.transport == nil {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC client is not initialized")
	}
	if ctx == nil {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC context is nil")
	}
	if strings.TrimSpace(endpoint) == "" {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC endpoint is empty")
	}
	if strings.TrimSpace(method) == "" {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC method is empty")
	}
	if len(params) == 0 {
		params = json.RawMessage(`[]`)
	}
	if err := validateJSONRPCParams(params); err != nil {
		return JSONRPCResponse{}, err
	}

	requestID := c.nextID.Add(1)
	body, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("encode JSON-RPC request: %w", err)
	}

	var decoded jsonRPCWireResponse
	if err := c.transport.Execute(ctx, http.MethodPost, endpoint, bytes.NewReader(body), &decoded); err != nil {
		return JSONRPCResponse{}, err
	}
	if decoded.JSONRPC != "2.0" {
		return JSONRPCResponse{}, fmt.Errorf("invalid JSON-RPC response version")
	}
	expectedID, err := json.Marshal(requestID)
	if err != nil {
		return JSONRPCResponse{}, fmt.Errorf("encode JSON-RPC request ID: %w", err)
	}
	if !bytes.Equal(bytes.TrimSpace(decoded.ID), expectedID) {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC response ID mismatch")
	}

	hasResult := decoded.Result != nil
	hasError := decoded.Error != nil
	if hasResult == hasError {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC response must contain exactly one of result or error")
	}
	if hasResult {
		return JSONRPCResponse{Result: cloneJSONRPCData(decoded.Result)}, nil
	}
	if bytes.Equal(bytes.TrimSpace(decoded.Error), []byte("null")) {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC error must be an object")
	}

	var wireError jsonRPCWireError
	if err := json.Unmarshal(decoded.Error, &wireError); err != nil {
		return JSONRPCResponse{}, fmt.Errorf("decode JSON-RPC error: %w", err)
	}
	if wireError.Code == nil || wireError.Message == nil {
		return JSONRPCResponse{}, fmt.Errorf("JSON-RPC error must contain code and message")
	}
	return JSONRPCResponse{Error: &JSONRPCError{
		Code:    *wireError.Code,
		Message: *wireError.Message,
		Data:    cloneJSONRPCData(wireError.Data),
	}}, nil
}

func cloneJSONRPCData(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func validateJSONRPCParams(params json.RawMessage) error {
	trimmed := bytes.TrimSpace(params)
	if !json.Valid(trimmed) {
		return fmt.Errorf("JSON-RPC params must be valid JSON")
	}
	if len(trimmed) == 0 || (trimmed[0] != '[' && trimmed[0] != '{') {
		return fmt.Errorf("JSON-RPC params must be an array or object")
	}
	return nil
}

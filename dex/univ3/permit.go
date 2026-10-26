package univ3

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/kernelflowlabs/wallet-lib/common/dexmodel"
)

const abiEIP2612 = `[
	{"inputs":[],"name":"DOMAIN_SEPARATOR","outputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"stateMutability":"view","type":"function"},
	{"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"nonces","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}
]`

var parsedEIP2612 = mustParsePermitABI()

func mustParsePermitABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(abiEIP2612))
	if err != nil {
		panic(fmt.Sprintf("univ3: parse EIP-2612 ABI: %v", err))
	}
	return parsed
}

type PermitRequest struct {
	TokenName    string `json:"tokenName"`
	TokenVersion string `json:"tokenVersion,omitempty"`
	ChainID      uint64 `json:"chainId"`
	Token        string `json:"token"`
	Owner        string `json:"owner"`
	Spender      string `json:"spender"`
	Value        string `json:"value"`
	Nonce        string `json:"nonce"`
	Deadline     string `json:"deadline"`
}

type PermitSignature struct {
	V uint8
	R [32]byte
	S [32]byte
}

type validatedPermit struct {
	request  PermitRequest
	token    common.Address
	owner    common.Address
	value    *big.Int
	nonce    *big.Int
	deadline *big.Int
}

func (c *Client) BuildPermitTypedData(request PermitRequest) (*apitypes.TypedData, error) {
	permit, err := c.validatePermitRequest(request)
	if err != nil {
		return nil, err
	}
	return c.permitTypedData(permit), nil
}

func (c *Client) permitTypedData(permit *validatedPermit) *apitypes.TypedData {
	chain := new(big.Int).SetUint64(c.config.ChainID)
	chainID := math.HexOrDecimal256(*chain)
	return &apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Permit": {
				{Name: "owner", Type: "address"},
				{Name: "spender", Type: "address"},
				{Name: "value", Type: "uint256"},
				{Name: "nonce", Type: "uint256"},
				{Name: "deadline", Type: "uint256"},
			},
		},
		PrimaryType: "Permit",
		Domain: apitypes.TypedDataDomain{
			Name: permit.request.TokenName, Version: permit.request.TokenVersion,
			ChainId: &chainID, VerifyingContract: permit.token.Hex(),
		},
		Message: apitypes.TypedDataMessage{
			"owner": permit.owner.Hex(), "spender": c.swapRouter.Hex(),
			"value": permit.value.String(), "nonce": permit.nonce.String(),
			"deadline": permit.deadline.String(),
		},
	}
}

func (c *Client) BuildPermitTypedDataJSON(request PermitRequest) ([]byte, error) {
	typedData, err := c.BuildPermitTypedData(request)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(typedData)
	if err != nil {
		return nil, fmt.Errorf("univ3: marshal EIP-2612 typed data: %w", err)
	}
	return encoded, nil
}

func (c *Client) ValidatePermit(ctx context.Context, request PermitRequest) error {
	if c == nil || c.eth == nil {
		return fmt.Errorf("univ3: client is not initialized")
	}
	if ctx == nil {
		return fmt.Errorf("univ3: context is nil")
	}
	permit, err := c.validatePermitRequest(request)
	if err != nil {
		return err
	}
	return c.validatePermitOnChain(ctx, permit)
}

func (c *Client) ValidatePermitSignature(ctx context.Context, request PermitRequest, signature []byte) error {
	if c == nil || c.eth == nil {
		return fmt.Errorf("univ3: client is not initialized")
	}
	if ctx == nil {
		return fmt.Errorf("univ3: context is nil")
	}
	permit, err := c.validatePermitRequest(request)
	if err != nil {
		return err
	}
	if _, err := c.validatePermitSigner(permit, signature); err != nil {
		return err
	}
	return c.validatePermitOnChain(ctx, permit)
}

func (c *Client) validatePermitOnChain(ctx context.Context, permit *validatedPermit) error {
	if err := c.ensureDeployment(ctx); err != nil {
		return err
	}
	code, err := c.eth.CodeAt(ctx, permit.token, nil)
	if err != nil {
		return fmt.Errorf("univ3: read permit token code: %w", err)
	}
	if len(code) == 0 {
		return fmt.Errorf("univ3: permit token %s has no contract code", permit.token.Hex())
	}
	typedData := c.permitTypedData(permit)
	expectedDomain, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return fmt.Errorf("univ3: hash EIP-2612 domain: %w", err)
	}
	domainData, err := parsedEIP2612.Pack("DOMAIN_SEPARATOR")
	if err != nil {
		return fmt.Errorf("univ3: pack EIP-2612 DOMAIN_SEPARATOR: %w", err)
	}
	domainRaw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &permit.token, Data: domainData}, nil)
	if err != nil {
		return fmt.Errorf("univ3: EIP-2612 DOMAIN_SEPARATOR(): %w", err)
	}
	domainValues, err := parsedEIP2612.Unpack("DOMAIN_SEPARATOR", domainRaw)
	if err != nil || len(domainValues) != 1 {
		if err == nil {
			err = fmt.Errorf("returned %d values", len(domainValues))
		}
		return fmt.Errorf("univ3: decode EIP-2612 DOMAIN_SEPARATOR: %w", err)
	}
	actualDomain, ok := domainValues[0].([32]byte)
	if !ok {
		return fmt.Errorf("univ3: EIP-2612 DOMAIN_SEPARATOR has type %T", domainValues[0])
	}
	if common.BytesToHash(expectedDomain) != common.BytesToHash(actualDomain[:]) {
		return fmt.Errorf("univ3: EIP-2612 DOMAIN_SEPARATOR does not match tokenName, tokenVersion, chainId, and token")
	}
	data, err := parsedEIP2612.Pack("nonces", permit.owner)
	if err != nil {
		return fmt.Errorf("univ3: pack EIP-2612 nonces: %w", err)
	}
	raw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &permit.token, Data: data}, nil)
	if err != nil {
		return fmt.Errorf("univ3: EIP-2612 nonces(%s): %w", permit.owner.Hex(), err)
	}
	values, err := parsedEIP2612.Unpack("nonces", raw)
	if err != nil || len(values) != 1 {
		if err == nil {
			err = fmt.Errorf("returned %d values", len(values))
		}
		return fmt.Errorf("univ3: decode EIP-2612 nonce: %w", err)
	}
	current, ok := values[0].(*big.Int)
	if !ok || current == nil {
		return fmt.Errorf("univ3: EIP-2612 nonce has type %T", values[0])
	}
	if current.Cmp(permit.nonce) != 0 {
		return fmt.Errorf("univ3: EIP-2612 nonce=%s, requested nonce=%s", current.String(), permit.nonce.String())
	}
	return nil
}

func (c *Client) validatePermitRequest(request PermitRequest) (*validatedPermit, error) {
	if c == nil {
		return nil, fmt.Errorf("univ3: client is not initialized")
	}
	request.TokenName = strings.TrimSpace(request.TokenName)
	request.TokenVersion = strings.TrimSpace(request.TokenVersion)
	request.Token = strings.TrimSpace(request.Token)
	request.Owner = strings.TrimSpace(request.Owner)
	request.Spender = strings.TrimSpace(request.Spender)
	request.Value = strings.TrimSpace(request.Value)
	request.Nonce = strings.TrimSpace(request.Nonce)
	request.Deadline = strings.TrimSpace(request.Deadline)
	if request.TokenName == "" {
		return nil, fmt.Errorf("univ3: EIP-2612 tokenName is required")
	}
	if request.TokenVersion == "" {
		request.TokenVersion = "1"
	}
	if request.ChainID != c.config.ChainID {
		return nil, fmt.Errorf("univ3: permit chainId=%d, configured chainId=%d", request.ChainID, c.config.ChainID)
	}
	token, err := requiredClientAddress("permit token", request.Token)
	if err != nil {
		return nil, err
	}
	if isClientNative(request.Token) {
		return nil, fmt.Errorf("univ3: native input does not support EIP-2612 permit")
	}
	owner, err := requiredClientAddress("permit owner", request.Owner)
	if err != nil {
		return nil, err
	}
	spender, err := requiredClientAddress("permit spender", request.Spender)
	if err != nil {
		return nil, err
	}
	if spender != c.swapRouter {
		return nil, fmt.Errorf("univ3: permit spender must be configured SwapRouter %s", c.swapRouter.Hex())
	}
	value, err := parsePermitUint256("value", request.Value, true)
	if err != nil {
		return nil, err
	}
	nonce, err := parsePermitUint256("nonce", request.Nonce, false)
	if err != nil {
		return nil, err
	}
	deadline, err := parsePermitUint256("deadline", request.Deadline, true)
	if err != nil {
		return nil, err
	}
	if deadline.Cmp(big.NewInt(c.now().Unix())) <= 0 {
		return nil, fmt.Errorf("univ3: permit deadline has expired")
	}
	request.Token = token.Hex()
	request.Owner = owner.Hex()
	request.Spender = c.swapRouter.Hex()
	request.Value = value.String()
	request.Nonce = nonce.String()
	request.Deadline = deadline.String()
	return &validatedPermit{request: request, token: token, owner: owner, value: value, nonce: nonce, deadline: deadline}, nil
}

func parsePermitUint256(name, raw string, positive bool) (*big.Int, error) {
	if len(raw) == 0 || len(raw) > 78 {
		qualifier := "a uint256"
		if positive {
			qualifier = "a positive uint256"
		}
		return nil, fmt.Errorf("univ3: permit %s must be %s decimal integer", name, qualifier)
	}
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.Sign() < 0 || value.BitLen() > 256 || positive && value.Sign() == 0 {
		qualifier := "a uint256"
		if positive {
			qualifier = "a positive uint256"
		}
		return nil, fmt.Errorf("univ3: permit %s must be %s decimal integer", name, qualifier)
	}
	return value, nil
}

func SplitPermitSignature(signature []byte) (*PermitSignature, error) {
	if len(signature) != crypto.SignatureLength {
		return nil, fmt.Errorf("univ3: permit signature must be 65 bytes, got %d", len(signature))
	}
	var result PermitSignature
	copy(result.R[:], signature[:32])
	copy(result.S[:], signature[32:64])
	result.V = signature[64]
	if result.V < 27 {
		result.V += 27
	}
	if result.V != 27 && result.V != 28 {
		return nil, fmt.Errorf("univ3: permit signature v must be 0, 1, 27, or 28")
	}
	r := new(big.Int).SetBytes(result.R[:])
	s := new(big.Int).SetBytes(result.S[:])
	if !crypto.ValidateSignatureValues(result.V-27, r, s, true) {
		return nil, fmt.Errorf("univ3: invalid permit signature values")
	}
	return &result, nil
}

func SplitPermitSignatureHex(signature string) (*PermitSignature, error) {
	signature = strings.TrimSpace(signature)
	if !strings.HasPrefix(signature, "0x") {
		return nil, fmt.Errorf("univ3: permit signature must be 0x-prefixed")
	}
	raw, err := hex.DecodeString(signature[2:])
	if err != nil {
		return nil, fmt.Errorf("univ3: decode permit signature: %w", err)
	}
	return SplitPermitSignature(raw)
}

func (c *Client) validatePermitSigner(permit *validatedPermit, signature []byte) (*PermitSignature, error) {
	split, err := SplitPermitSignature(signature)
	if err != nil {
		return nil, err
	}
	typedData := c.permitTypedData(permit)
	digest, _, err := apitypes.TypedDataAndHash(*typedData)
	if err != nil {
		return nil, fmt.Errorf("univ3: hash EIP-2612 permit: %w", err)
	}
	normalized := make([]byte, crypto.SignatureLength)
	copy(normalized[:32], split.R[:])
	copy(normalized[32:64], split.S[:])
	normalized[64] = split.V - 27
	publicKey, err := crypto.SigToPub(digest, normalized)
	if err != nil {
		return nil, fmt.Errorf("univ3: recover EIP-2612 signer: %w", err)
	}
	recovered := crypto.PubkeyToAddress(*publicKey)
	if recovered != permit.owner {
		return nil, fmt.Errorf("univ3: EIP-2612 signature owner=%s, requested owner=%s", recovered.Hex(), permit.owner.Hex())
	}
	return split, nil
}

func (c *Client) EncodeSelfPermit(ctx context.Context, request PermitRequest, signature []byte) ([]byte, error) {
	return c.encodePermitCall(ctx, "selfPermit", request, signature)
}

func (c *Client) EncodeSelfPermitIfNecessary(ctx context.Context, request PermitRequest, signature []byte) ([]byte, error) {
	return c.encodePermitCall(ctx, "selfPermitIfNecessary", request, signature)
}

func (c *Client) encodePermitCall(ctx context.Context, method string, request PermitRequest, signature []byte) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("univ3: context is nil")
	}
	permit, err := c.validatePermitRequest(request)
	if err != nil {
		return nil, err
	}
	split, err := c.validatePermitSigner(permit, signature)
	if err != nil {
		return nil, err
	}
	if err := c.validatePermitOnChain(ctx, permit); err != nil {
		return nil, err
	}
	data, err := parsedV1.Pack(method, permit.token, permit.value, permit.deadline, split.V, split.R, split.S)
	if err != nil {
		return nil, fmt.Errorf("univ3: encode %s: %w", method, err)
	}
	return data, nil
}

func (c *Client) EncodePermitMulticall(ctx context.Context, swapCalldata []byte, request PermitRequest, signature []byte, ifNecessary bool) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("univ3: client is not initialized")
	}
	if ctx == nil {
		return nil, fmt.Errorf("univ3: context is nil")
	}
	infos, err := DecodeSwapInfos(swapCalldata, RouterVersionV1)
	if err != nil {
		return nil, fmt.Errorf("univ3: validate permit swap calldata: %w", err)
	}
	if len(infos) != 1 {
		return nil, fmt.Errorf("univ3: permit helper requires exactly one swap, got %d", len(infos))
	}
	permit, err := c.validatePermitRequest(request)
	if err != nil {
		return nil, err
	}
	if infos[0].TokenIn != permit.token {
		return nil, fmt.Errorf("univ3: permit token %s does not match swap input %s", permit.token.Hex(), infos[0].TokenIn.Hex())
	}
	if permit.value.Cmp(infos[0].AmountIn) < 0 {
		return nil, fmt.Errorf("univ3: permit value %s is below swap amount %s", permit.value.String(), infos[0].AmountIn.String())
	}
	method := "selfPermit"
	if ifNecessary {
		method = "selfPermitIfNecessary"
	}
	permitCall, err := c.encodePermitCall(ctx, method, permit.request, signature)
	if err != nil {
		return nil, err
	}
	calls := [][]byte{permitCall}
	if len(swapCalldata) >= 4 && hex.EncodeToString(swapCalldata[:4]) == SelectorMulticall {
		values, err := parsedV1.Methods["multicall"].Inputs.Unpack(swapCalldata[4:])
		if err != nil || len(values) != 1 {
			if err == nil {
				err = fmt.Errorf("returned %d values", len(values))
			}
			return nil, fmt.Errorf("univ3: unpack existing multicall: %w", err)
		}
		inner, ok := values[0].([][]byte)
		if !ok {
			return nil, fmt.Errorf("univ3: existing multicall data has type %T", values[0])
		}
		calls = append(calls, inner...)
	} else {
		calls = append(calls, swapCalldata)
	}
	data, err := parsedV1.Pack("multicall", calls)
	if err != nil {
		return nil, fmt.Errorf("univ3: encode permit multicall: %w", err)
	}
	return data, nil
}

func (c *Client) AttachPermit(ctx context.Context, route *dexmodel.DexRoute, request PermitRequest, signature []byte, ifNecessary bool) (*dexmodel.DexTx, error) {
	if ctx == nil {
		return nil, fmt.Errorf("univ3: context is nil")
	}
	if route == nil || route.TxData == nil || route.ApprovalData == nil {
		return nil, fmt.Errorf("univ3: permit requires an ERC-20-input quoted route")
	}
	if !c.IsConfiguredRouter(route.TxData.To) || !c.IsConfiguredRouter(route.ApprovalData.Spender) {
		return nil, fmt.Errorf("univ3: route spender or transaction target is not the configured SwapRouter")
	}
	permit, err := c.validatePermitRequest(request)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(route.ApprovalData.Token, permit.token.Hex()) {
		return nil, fmt.Errorf("univ3: permit token does not match route approval token")
	}
	approvalAmount, ok := new(big.Int).SetString(route.ApprovalData.Amount, 10)
	if !ok || approvalAmount.Sign() <= 0 || permit.value.Cmp(approvalAmount) < 0 {
		return nil, fmt.Errorf("univ3: permit value does not cover route approval amount")
	}
	if route.ExpiresAt <= 0 || permit.deadline.Cmp(big.NewInt(route.ExpiresAt)) < 0 {
		return nil, fmt.Errorf("univ3: permit deadline must cover the quoted swap deadline")
	}
	if route.TxData.Value != "" && route.TxData.Value != "0" {
		return nil, fmt.Errorf("univ3: native-input routes must not use EIP-2612 permit")
	}
	rawData := strings.TrimSpace(route.TxData.Data)
	if !strings.HasPrefix(rawData, "0x") {
		return nil, fmt.Errorf("univ3: route transaction data must be 0x-prefixed")
	}
	swapCalldata, err := hex.DecodeString(rawData[2:])
	if err != nil {
		return nil, fmt.Errorf("univ3: decode route transaction data: %w", err)
	}
	data, err := c.EncodePermitMulticall(ctx, swapCalldata, permit.request, signature, ifNecessary)
	if err != nil {
		return nil, err
	}
	out := *route.TxData
	out.Data = "0x" + hex.EncodeToString(data)
	return &out, nil
}

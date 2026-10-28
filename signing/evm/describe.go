package evm

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/kernelflowlabs/wallet-lib/contracts/permit2"
)

const (
	KindNativeTransfer        = "native_transfer"
	KindErc20Transfer         = "erc20_transfer"
	KindTransferFromAmbiguous = "transfer_from_ambiguous"
	KindErc20Approve          = "erc20_approve"
	KindErc20IncreaseAllow    = "erc20_increase_allowance"
	KindErc20DecreaseAllow    = "erc20_decrease_allowance"
	KindNftSetApprovalForAll  = "nft_set_approval_for_all"
	KindNftSafeTransfer       = "nft_safe_transfer"
	KindPermit2Approve        = "permit2_approve"
	KindContractCall          = "contract_call"
	KindContractCreation      = "contract_creation"
)

const (
	WarnContractCreation   = "contract_creation"
	WarnValueWithCall      = "value_with_call"
	WarnOperatorFullAccess = "operator_full_control"
	WarnUnlimitedApproval  = "unlimited_approval"
	WarnZeroAddress        = "zero_address"
	WarnErc20OrErc721      = "erc20_or_erc721"
	WarnUnknownSelector    = "unknown_selector"
)

type EvmCallRequest struct {
	ChainID string `json:"chainId"`
	From    string `json:"from"`
	To      string `json:"to"`
	Value   string `json:"value"`
	Data    string `json:"data"`
}

type EvmCallDescription struct {
	Kind       string `json:"kind"`
	Selector   string `json:"selector,omitempty"`
	To         string `json:"to,omitempty"`
	Token      string `json:"token,omitempty"`
	Spender    string `json:"spender,omitempty"`
	Operator   string `json:"operator,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Recipient  string `json:"recipient,omitempty"`
	Amount     string `json:"amount,omitempty"`
	TokenID    string `json:"tokenId,omitempty"`
	Value      string `json:"value,omitempty"`
	Expiration string `json:"expiration,omitempty"`

	Unlimited bool `json:"unlimited,omitempty"`
	Revoke    bool `json:"revoke,omitempty"`
	Approved  bool `json:"approved,omitempty"`

	Warnings    []string `json:"warnings"`
	DecodeError string   `json:"decodeError,omitempty"`
}

var maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
var unlimitedFloor = new(big.Int).Lsh(big.NewInt(1), 255)

// DescribeEvmCall turns an eth_sendTransaction-shaped request into what the
// confirmation screen should say. It never reaches the network and never sees
// a key: anything it cannot decode comes back as contract_call with the raw
// selector, not as a guess.
func DescribeEvmCall(requestJSON string) string {
	var req EvmCallRequest
	if err := json.Unmarshal([]byte(requestJSON), &req); err != nil {
		return marshalDescription(&EvmCallDescription{
			Kind:        KindContractCall,
			DecodeError: "request is not valid json",
		})
	}
	return marshalDescription(describeEvmCall(&req))
}

func describeEvmCall(req *EvmCallRequest) *EvmCallDescription {
	out := &EvmCallDescription{Warnings: []string{}}

	value, ok := parseQuantity(req.Value)
	if !ok {
		out.DecodeError = "value is not a number"
		value = big.NewInt(0)
	}
	if value.Sign() > 0 {
		out.Value = value.String()
	}

	to := strings.TrimSpace(req.To)
	if to != "" {
		normalized, ok := NormalizeAddress(to)
		if !ok {
			out.Kind = KindContractCall
			out.DecodeError = "to is not an address"
			return out
		}
		out.To = normalized
	}

	data, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(req.Data), "0x"))
	if err != nil {
		out.Kind = KindContractCall
		out.DecodeError = "data is not hex"
		return out
	}

	if to == "" {
		out.Kind = KindContractCreation
		out.Warnings = append(out.Warnings, WarnContractCreation)
		return out
	}
	if len(data) == 0 {
		out.Kind = KindNativeTransfer
		out.Recipient = out.To
		out.Amount = value.String()
		return out
	}
	if value.Sign() > 0 {
		out.Warnings = append(out.Warnings, WarnValueWithCall)
	}
	if len(data) < 4 {
		out.Kind = KindContractCall
		out.DecodeError = "calldata is shorter than a selector"
		return out
	}

	out.Selector = "0x" + hex.EncodeToString(data[:4])
	out.Token = out.To
	decodeEvmCalldata(out, data)
	return out
}

func decodeEvmCalldata(out *EvmCallDescription, data []byte) {
	args := data[4:]
	switch out.Selector {
	case "0xa9059cbb": // transfer(address,uint256)
		if !requireWords(out, args, 2) {
			return
		}
		out.Kind = KindErc20Transfer
		out.Recipient = wordAddress(args, 0)
		out.Amount = wordUint(args, 1).String()
		flagZeroAddress(out, out.Recipient)

	case "0x23b872dd": // transferFrom(address,address,uint256)
		if !requireWords(out, args, 3) {
			return
		}
		// The third word is an ERC-20 amount or an ERC-721 tokenId. Calldata
		// alone cannot tell them apart, so both are reported and the caller
		// decides from the token it already knows about.
		out.Kind = KindTransferFromAmbiguous
		out.Owner = wordAddress(args, 0)
		out.Recipient = wordAddress(args, 1)
		out.Amount = wordUint(args, 2).String()
		out.TokenID = out.Amount
		out.Warnings = append(out.Warnings, WarnErc20OrErc721)
		flagZeroAddress(out, out.Recipient)

	case "0x095ea7b3": // approve(address,uint256)
		if !requireWords(out, args, 2) {
			return
		}
		out.Kind = KindErc20Approve
		out.Spender = wordAddress(args, 0)
		amount := wordUint(args, 1)
		out.Amount = amount.String()
		out.TokenID = out.Amount
		out.Warnings = append(out.Warnings, WarnErc20OrErc721)
		if amount.Sign() == 0 {
			out.Revoke = true
		} else if amount.Cmp(unlimitedFloor) >= 0 {
			out.Unlimited = true
			out.Warnings = append(out.Warnings, WarnUnlimitedApproval)
		}
		flagZeroAddress(out, out.Spender)

	case "0x39509351": // increaseAllowance(address,uint256)
		if !requireWords(out, args, 2) {
			return
		}
		out.Kind = KindErc20IncreaseAllow
		out.Spender = wordAddress(args, 0)
		amount := wordUint(args, 1)
		out.Amount = amount.String()
		if amount.Cmp(unlimitedFloor) >= 0 {
			out.Unlimited = true
			out.Warnings = append(out.Warnings, WarnUnlimitedApproval)
		}
		flagZeroAddress(out, out.Spender)

	case "0xa457c2d7": // decreaseAllowance(address,uint256)
		if !requireWords(out, args, 2) {
			return
		}
		out.Kind = KindErc20DecreaseAllow
		out.Spender = wordAddress(args, 0)
		out.Amount = wordUint(args, 1).String()

	case "0xa22cb465": // setApprovalForAll(address,bool)
		if !requireWords(out, args, 2) {
			return
		}
		out.Kind = KindNftSetApprovalForAll
		out.Operator = wordAddress(args, 0)
		out.Approved = wordUint(args, 1).Sign() != 0
		if out.Approved {
			out.Warnings = append(out.Warnings, WarnOperatorFullAccess)
		} else {
			out.Revoke = true
		}
		flagZeroAddress(out, out.Operator)

	case "0x42842e0e", "0xb88d4fde": // ERC-721 safeTransferFrom
		if !requireWords(out, args, 3) {
			return
		}
		out.Kind = KindNftSafeTransfer
		out.Owner = wordAddress(args, 0)
		out.Recipient = wordAddress(args, 1)
		out.TokenID = wordUint(args, 2).String()
		flagZeroAddress(out, out.Recipient)

	case "0xf242432a": // ERC-1155 safeTransferFrom(address,address,uint256,uint256,bytes)
		if !requireWords(out, args, 5) {
			return
		}
		out.Kind = KindNftSafeTransfer
		out.Owner = wordAddress(args, 0)
		out.Recipient = wordAddress(args, 1)
		out.TokenID = wordUint(args, 2).String()
		out.Amount = wordUint(args, 3).String()
		flagZeroAddress(out, out.Recipient)

	case "0x2eb2c2d6": // ERC-1155 safeBatchTransferFrom
		if !requireWords(out, args, 5) {
			return
		}
		out.Kind = KindNftSafeTransfer
		out.Owner = wordAddress(args, 0)
		out.Recipient = wordAddress(args, 1)
		flagZeroAddress(out, out.Recipient)

	case "0x87517c45": // Permit2 approve(address,address,uint160,uint48)
		if !requireWords(out, args, 4) {
			return
		}
		if !strings.EqualFold(out.To, permit2.Address) {
			out.Kind = KindContractCall
			out.Warnings = append(out.Warnings, WarnUnknownSelector)
			return
		}
		out.Kind = KindPermit2Approve
		out.Token = wordAddress(args, 0)
		out.Spender = wordAddress(args, 1)
		amount := wordUint(args, 2)
		out.Amount = amount.String()
		out.Expiration = wordUint(args, 3).String()
		// Permit2 stores the allowance in a uint160, so its "forever" value is
		// 2^160-1 rather than 2^256-1.
		if amount.Cmp(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 160), big.NewInt(1))) == 0 {
			out.Unlimited = true
			out.Warnings = append(out.Warnings, WarnUnlimitedApproval)
		}
		if amount.Sign() == 0 {
			out.Revoke = true
		}
		flagZeroAddress(out, out.Spender)

	default:
		out.Kind = KindContractCall
		out.Warnings = append(out.Warnings, WarnUnknownSelector)
	}
}

func requireWords(out *EvmCallDescription, args []byte, n int) bool {
	if len(args) < n*32 {
		out.Kind = KindContractCall
		out.DecodeError = fmt.Sprintf("calldata holds %d bytes of arguments, %s needs %d", len(args), out.Selector, n*32)
		return false
	}
	return true
}

func wordAddress(args []byte, index int) string {
	word := args[index*32 : (index+1)*32]
	return common.BytesToAddress(word[12:]).Hex()
}

func wordUint(args []byte, index int) *big.Int {
	return new(big.Int).SetBytes(args[index*32 : (index+1)*32])
}

func flagZeroAddress(out *EvmCallDescription, address string) {
	if address != "" && IsZeroAddress(address) {
		out.Warnings = append(out.Warnings, WarnZeroAddress)
	}
}

// parseQuantity accepts both the 0x form JSON-RPC uses and a plain decimal
// string, because callers reach this from both directions.
func parseQuantity(s string) (*big.Int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return big.NewInt(0), true
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, ok := new(big.Int).SetString(s[2:], 16)
		return v, ok
	}
	v, ok := new(big.Int).SetString(s, 10)
	return v, ok
}

func marshalDescription(d *EvmCallDescription) string {
	if d.Warnings == nil {
		d.Warnings = []string{}
	}
	out, err := json.Marshal(d)
	if err != nil {
		return `{"kind":"contract_call","warnings":[],"decodeError":"cannot encode description"}`
	}
	return string(out)
}

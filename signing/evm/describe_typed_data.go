package evm

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/kernelflowlabs/wallet-lib/contracts/permit2"
)

const (
	KindErc2612Permit       = "erc2612_permit"
	KindDaiPermit           = "dai_permit"
	KindPermit2Single       = "permit2_single"
	KindPermit2Batch        = "permit2_batch"
	KindPermit2TransferFrom = "permit2_transfer_from"
	KindSeaportOrder        = "seaport_order"
	KindTypedDataOther      = "other"
)

const (
	WarnMissingChainID       = "missing_chain_id"
	WarnMissingVerifyingAddr = "missing_verifying_contract"
	WarnLongExpiry           = "long_expiry"
	WarnMarketplaceListing   = "marketplace_listing"
	WarnUnreachableTypes     = "unreachable_types"
)

const oneYearSeconds = 365 * 24 * 60 * 60

type TypedDataDomainView struct {
	Name              string `json:"name,omitempty"`
	Version           string `json:"version,omitempty"`
	ChainID           string `json:"chainId,omitempty"`
	VerifyingContract string `json:"verifyingContract,omitempty"`
	Salt              string `json:"salt,omitempty"`
}

type TypedDataDescription struct {
	Kind        string              `json:"kind"`
	PrimaryType string              `json:"primaryType,omitempty"`
	Digest      string              `json:"digest,omitempty"`
	Domain      TypedDataDomainView `json:"domain"`

	Token    string `json:"token,omitempty"`
	Owner    string `json:"owner,omitempty"`
	Spender  string `json:"spender,omitempty"`
	Amount   string `json:"amount,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
	Deadline string `json:"deadline,omitempty"`

	OfferCount         int `json:"offerCount,omitempty"`
	ConsiderationCount int `json:"considerationCount,omitempty"`

	Unlimited bool `json:"unlimited,omitempty"`

	Warnings    []string `json:"warnings"`
	DecodeError string   `json:"decodeError,omitempty"`
}

// DescribeTypedData explains an eth_signTypedData payload. The digest it
// reports is produced by the same HashTypedDataJSON the signer uses, so the
// screen and the signature cannot describe different things.
//
// The v1 array form is rejected rather than guessed at.
func DescribeTypedData(typedDataJSON string) string {
	out := &TypedDataDescription{Kind: KindTypedDataOther, Warnings: []string{}}

	trimmed := strings.TrimSpace(typedDataJSON)
	if strings.HasPrefix(trimmed, "[") {
		out.DecodeError = "eth_signTypedData v1 (array form) is not supported"
		return marshalTypedDataDescription(out)
	}

	var td apitypes.TypedData
	if err := json.Unmarshal([]byte(typedDataJSON), &td); err != nil {
		out.DecodeError = "typed data is not valid json"
		return marshalTypedDataDescription(out)
	}

	digest, err := HashTypedDataJSON([]byte(typedDataJSON))
	if err != nil {
		out.DecodeError = err.Error()
		return marshalTypedDataDescription(out)
	}
	out.Digest = "0x" + hex.EncodeToString(digest)
	out.PrimaryType = td.PrimaryType
	out.Domain = domainView(td.Domain)

	if td.Domain.ChainId == nil {
		out.Warnings = append(out.Warnings, WarnMissingChainID)
	}
	if td.Domain.VerifyingContract == "" {
		out.Warnings = append(out.Warnings, WarnMissingVerifyingAddr)
	} else if IsZeroAddress(td.Domain.VerifyingContract) {
		out.Warnings = append(out.Warnings, WarnZeroAddress)
	}
	if unreachable := unreachableTypes(td); len(unreachable) > 0 {
		out.Warnings = append(out.Warnings, WarnUnreachableTypes)
	}

	classifyTypedData(out, &td)
	return marshalTypedDataDescription(out)
}

func classifyTypedData(out *TypedDataDescription, td *apitypes.TypedData) {
	msg := td.Message
	isPermit2 := strings.EqualFold(td.Domain.VerifyingContract, permit2.Address)

	switch td.PrimaryType {
	case "Permit":
		if _, hasAllowed := msg["allowed"]; hasAllowed {
			out.Kind = KindDaiPermit
			out.Owner = messageString(msg, "holder")
			out.Spender = messageString(msg, "spender")
			out.Nonce = messageString(msg, "nonce")
			out.Deadline = messageString(msg, "expiry")
			// DAI's permit has no amount: "allowed" grants the spender the
			// whole balance until the expiry.
			out.Unlimited = messageBool(msg, "allowed")
		} else {
			out.Kind = KindErc2612Permit
			out.Owner = messageString(msg, "owner")
			out.Spender = messageString(msg, "spender")
			out.Amount = messageString(msg, "value")
			out.Nonce = messageString(msg, "nonce")
			out.Deadline = messageString(msg, "deadline")
			out.Unlimited = isUnlimitedAmount(out.Amount)
		}
		out.Token = td.Domain.VerifyingContract

	case "PermitSingle":
		if !isPermit2 {
			break
		}
		out.Kind = KindPermit2Single
		out.Spender = messageString(msg, "spender")
		out.Deadline = messageString(msg, "sigDeadline")
		if details, ok := msg["details"].(map[string]interface{}); ok {
			out.Token = messageString(details, "token")
			out.Amount = messageString(details, "amount")
			out.Nonce = messageString(details, "nonce")
			out.Unlimited = isUnlimitedUint160(out.Amount)
		}

	case "PermitBatch":
		if !isPermit2 {
			break
		}
		out.Kind = KindPermit2Batch
		out.Spender = messageString(msg, "spender")
		out.Deadline = messageString(msg, "sigDeadline")

	case "PermitTransferFrom", "PermitBatchTransferFrom", "PermitWitnessTransferFrom":
		if !isPermit2 {
			break
		}
		out.Kind = KindPermit2TransferFrom
		out.Spender = messageString(msg, "spender")
		out.Nonce = messageString(msg, "nonce")
		out.Deadline = messageString(msg, "deadline")
		if permitted, ok := msg["permitted"].(map[string]interface{}); ok {
			out.Token = messageString(permitted, "token")
			out.Amount = messageString(permitted, "amount")
		}

	case "OrderComponents":
		out.Kind = KindSeaportOrder
		out.Owner = messageString(msg, "offerer")
		out.OfferCount = messageLen(msg, "offer")
		out.ConsiderationCount = messageLen(msg, "consideration")
		out.Deadline = messageString(msg, "endTime")
		out.Warnings = append(out.Warnings, WarnMarketplaceListing)
	}

	if out.Kind == "" {
		out.Kind = KindTypedDataOther
	}
	if flagged := deadlineWarning(out.Deadline); flagged {
		out.Warnings = append(out.Warnings, WarnLongExpiry)
	}
	if out.Spender != "" && IsZeroAddress(out.Spender) {
		out.Warnings = append(out.Warnings, WarnZeroAddress)
	}
}

// unreachableTypes lists declared struct types that the primary type does not
// reach. They are not hashed, so anything shown from them would not be part of
// what gets signed.
func unreachableTypes(td apitypes.TypedData) []string {
	seen := map[string]bool{"EIP712Domain": true}
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		for _, field := range td.Types[name] {
			walk(strings.TrimSuffix(strings.SplitN(field.Type, "[", 2)[0], "[]"))
		}
	}
	walk(td.PrimaryType)

	var unreachable []string
	for name := range td.Types {
		if !seen[name] {
			unreachable = append(unreachable, name)
		}
	}
	return unreachable
}

func domainView(d apitypes.TypedDataDomain) TypedDataDomainView {
	view := TypedDataDomainView{
		Name:              d.Name,
		Version:           d.Version,
		VerifyingContract: d.VerifyingContract,
		Salt:              d.Salt,
	}
	if d.ChainId != nil {
		view.ChainID = (*big.Int)(d.ChainId).String()
	}
	return view
}

func messageString(m map[string]interface{}, key string) string {
	switch v := m[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return new(big.Float).SetFloat64(v).Text('f', 0)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func messageBool(m map[string]interface{}, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	case float64:
		return v != 0
	default:
		return false
	}
}

func messageLen(m map[string]interface{}, key string) int {
	if list, ok := m[key].([]interface{}); ok {
		return len(list)
	}
	return 0
}

func isUnlimitedAmount(s string) bool {
	v, ok := new(big.Int).SetString(s, 10)
	return ok && v.Cmp(unlimitedFloor) >= 0
}

func isUnlimitedUint160(s string) bool {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return false
	}
	return v.Cmp(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 160), big.NewInt(1))) == 0
}

// deadlineWarning flags a deadline that is effectively never: zero, or more
// than a year away. Zero counts because several permit implementations read it
// as "no expiry" rather than "already expired".
func deadlineWarning(deadline string) bool {
	if deadline == "" {
		return false
	}
	v, ok := new(big.Int).SetString(deadline, 10)
	if !ok {
		return false
	}
	if v.Sign() == 0 {
		return true
	}
	return v.Cmp(big.NewInt(time.Now().Unix()+oneYearSeconds)) > 0
}

func marshalTypedDataDescription(d *TypedDataDescription) string {
	if d.Warnings == nil {
		d.Warnings = []string{}
	}
	out, err := json.Marshal(d)
	if err != nil {
		return `{"kind":"other","warnings":[],"decodeError":"cannot encode description"}`
	}
	return string(out)
}

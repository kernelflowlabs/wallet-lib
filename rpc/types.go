package chainrpc

import (
	"encoding/json"
	"fmt"
)

type (
	TxTransfers struct {
		Hash          string           `json:"hash"`
		Rejected      bool             `json:"rejected"`
		ErrMsg        string           `json:"errMsg"`
		Transfers     []*Transfer      `json:"transfers"`
		BalanceChange []*BalanceChange `json:"balanceChange"`
	}
	Transfer struct {
		Sender          string `json:"sender"`
		Recipient       string `json:"recipient"`
		Amount          string `json:"amount"`
		ContractAddress string `json:"contractAddress"`
		Memo            string `json:"memo,omitempty"`
	}
	BalanceChange struct {
		Address         string `json:"address"`
		ContractAddress string `json:"contractAddress"`
		Change          string `json:"change"`
	}
	TxResult struct {
		Status  string   `json:"status"`
		Height  string   `json:"height"`
		Time    string   `json:"time"`
		GasUsed string   `json:"gasUsed"`
		ErrMsg  string   `json:"errMsg"`
		Logs    []EvmLog `json:"logs,omitempty"`
	}
	EvmLog struct {
		Index       uint64 `json:"index"`
		Address     string `json:"address"`
		Topics      string `json:"topics"`
		Data        string `json:"data"`
		BlockNumber uint64 `json:"blockNumber"`
		TxHash      string `json:"txHash,omitempty"`
	}
	BasicEvmTx struct {
		Nonce   string `json:"nonce"`
		Hash    string `json:"hash"`
		From    string `json:"from"`
		To      string `json:"to"`
		Payload string `json:"input"`
	}
	TokenInfo struct {
		Name     string `json:"name"`
		Symbol   string `json:"symbol"`
		Decimals string `json:"decimals"`
	}
)

func (in *EvmLog) String() string {
	b, err := json.Marshal(*in)
	if err != nil {
		return fmt.Sprintf("%+v", *in)
	}
	return string(b)
}

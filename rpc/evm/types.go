package evm

import "github.com/ethereum/go-ethereum/common/hexutil"

type (
	RpcTransaction struct {
		Hash        string `json:"hash"`
		BlockNumber string `json:"blockNumber"`
		From        string `json:"from"`
		To          string `json:"to"`
		Nonce       string `json:"nonce"`
		GasPrice    string `json:"gasPrice"`
		GasLimit    string `json:"gas"`
		Value       string `json:"value"`
		Payload     string `json:"input"`
		BlockHash   string `json:"blockHash,omitempty"`
	}

	RpcReceipt struct {
		BlockNumber     string    `json:"blockNumber"`
		ContractAddress string    `json:"contractAddress"`
		GasUsed         string    `json:"gasUsed"`
		Status          string    `json:"status"`
		Logs            []*RpcLog `json:"logs"`
	}
	RpcLog struct {
		Index   hexutil.Uint64 `json:"logIndex"`
		Address string         `json:"address"`
		Topics  []string       `json:"topics"`
		Data    string         `json:"data"`
	}
	RpcInternalTx struct {
		Action *struct {
			CallType string `json:"callType"`
			From     string `json:"from"`
			To       string `json:"to"`
			Value    string `json:"value"`
		} `json:"action"`
		Result *struct {
			GasUsed string `json:"gasUsed"`
			Output  string `json:"output"`
		} `json:"result"`
		Type         string `json:"type"`
		Error        string `json:"error,omitempty"`
		TraceAddress []int  `json:"traceAddress"`
	}
	RpcBlock struct {
		RpcBlockHeader `json:"blockHeader"`
		RpcBlockBody   `json:"blockBody"`
	}

	RpcBlockHeader struct {
		Hash       string `json:"hash"`
		ParentHash string `json:"parentHash"`
		Difficulty string `json:"difficulty"`
		Number     string `json:"number"`
		Time       string `json:"timestamp"`
		Size       string `json:"size"`
		Nonce      string `json:"nonce"`
		Miner      string `json:"miner"`
	}

	RpcBlockBody struct {
		Transactions []RpcTransaction `json:"transactions"`
	}
)

const opGasPriceOracleABI = `[{"inputs":[{"internalType":"bytes","name":"_data","type":"bytes"}],"name":"getL1Fee","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}]`
const opGasPriceOracleAddress = "0x420000000000000000000000000000000000000F"

const (
	ERC20TransferEvent      = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	SignatureTransferMethod = "a9059cbb"
	SignatureBalanceOf      = "70a08231"
)

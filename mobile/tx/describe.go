//go:build evm || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/signing/evm"
)

func DescribeEvmCall(requestJSON string) string {
	return evm.DescribeEvmCall(requestJSON)
}

func DescribeTypedData(typedDataJSON string) string {
	return evm.DescribeTypedData(typedDataJSON)
}

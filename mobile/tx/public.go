package tx

import (
	"github.com/kernelflowlabs/wallet-lib/crypto"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const (
	MobileTxTypeTransfer        = signing.TxTypeTransfer
	MobileTxTypeMint            = signing.TxTypeMint
	MobileTxTypeBurn            = signing.TxTypeBurn
	MobileTxTypeContractCall    = signing.TxTypeContractCall
	MobileTxTypeAccountActivate = signing.TxTypeAccountActivate
	MobileTxTypeKrc20Commit     = signing.TxTypeKrc20Commit
	MobileTxTypeKrc20Reveal     = signing.TxTypeKrc20Reveal
)

const (
	MobileTxStatusUnknown    = signing.TxStatusUnknown
	MobileTxStatusPending    = signing.TxStatusPending
	MobileTxStatusVerified   = signing.TxStatusVerified
	MobileTxStatusSucceeded  = signing.TxStatusSucceeded
	MobileTxStatusFailed     = signing.TxStatusFailed
	MobileTxStatusDropped    = signing.TxStatusDropped
	MobileTxStatusWaitVerify = signing.TxStatusWaitVerify
	MobileTxStatusRefunding  = signing.TxStatusRefunding
	MobileTxStatusRefunded   = signing.TxStatusRefunded
)

const (
	MobileMagicAddressForZeroEVM          = signing.MagicAddressForZeroEVM
	MobileMagicContactAddressForNative    = signing.MagicContactAddressForNative
	MobileMagicContactAddressForNativeTRX = signing.MagicContactAddressForNativeTRX
	MobileMagicContactAddressForNativeSOL = signing.MagicContactAddressForNativeSOL
	MobileMagicNumberForMaxAmount         = signing.MagicNumberForMaxAmount
)

type (
	MobileUtxoList signing.UtxoList
	MobileUtxoInfo signing.UtxoInfo
)

func NewMobileUtxoList() *MobileUtxoList {
	return &MobileUtxoList{
		List: make([]*signing.UtxoInfo, 0),
	}
}

func (m *MobileUtxoList) SerializeFromStr(jsonStr string) error {
	return (*signing.UtxoList)(m).SerializeFromStr(jsonStr)
}

func (m *MobileUtxoList) AddUtxoInfo(info *MobileUtxoInfo) {
	(*signing.UtxoList)(m).AddUtxoInfo((*signing.UtxoInfo)(info))
}

func (m *MobileUtxoList) SelectUtxo(targetValue string) error {
	return (*signing.UtxoList)(m).SelectUtxo(targetValue)
}

func (m *MobileUtxoList) CalcValue() string {
	return (*signing.UtxoList)(m).CalcValue()
}

func (m *MobileUtxoList) String() string {
	return (*signing.UtxoList)(m).String()
}

func WipeByte(b []byte) {
	crypto.Wipe(b)
}

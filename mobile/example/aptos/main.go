package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	aptosrpc "github.com/kernelflowlabs/wallet-lib/rpc/aptos"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/637'/0'/0'/0'"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://fullnode.mainnet.aptoslabs.com"
	network  = "1"
)

func main() {
	ctx := context.Background()
	_ = network

	a, err := acc.NewAptFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1"
	gasLimit := "2000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := aptosrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	sequence, err := h.InquireChain(ctx, "getNonce", sender)
	if err != nil {
		log.Fatalf("getNonce: %v", err)
	}
	gasPrice, err := h.InquireChain(ctx, "getGasPrice", "")
	if err != nil {
		log.Fatalf("getGasPrice: %v", err)
	}
	ledgerInfo, err := h.InquireChain(ctx, "getLedgerInfo", "")
	if err != nil {
		log.Fatalf("getLedgerInfo: %v", err)
	}
	var ledger struct {
		ChainId             string `json:"chainId"`
		ExpirationTimestamp string `json:"expirationTimestamp"`
	}
	if err := json.Unmarshal([]byte(ledgerInfo), &ledger); err != nil {
		log.Fatalf("decode ledger info: %v", err)
	}
	expiration := strconv.FormatInt(time.Now().Unix()+600, 10)
	fmt.Printf("2. fetched from rpc: sequence=%s gasPrice=%s chainId=%s\n", sequence, gasPrice, ledger.ChainId)

	b := tx.NewAptTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetSenderPubKey(a.PublicKeyHex()).
		SetRecipient(recipient).
		SetAmount(amount).
		SetNonce(sequence).
		SetGasPrice(gasPrice).
		SetGasLimit(gasLimit).
		SetChainId(ledger.ChainId).
		SetExpirationTimestamp(expiration)
	if err := b.Build(); err != nil {
		log.Fatalf("build: %v", err)
	}
	fmt.Println("3. built tx, sigHash:", b.SigHash())

	sig, err := b.Sign(a.PrivateKey())
	if err != nil {
		log.Fatalf("sign: %v", err)
	}
	signedHex, err := b.ConcatSignature(sig)
	if err != nil {
		log.Fatalf("concat signature: %v", err)
	}
	fmt.Println("4. signed raw tx:", signedHex)

	txHash, err := h.SendTx(ctx, signedHex)
	if err != nil {
		log.Fatalf("send tx: %v", err)
	}
	fmt.Println("5. broadcasted, txHash:", txHash)

	fmt.Println("6. waiting for confirmation...")
	for i := 0; i < 40; i++ {
		time.Sleep(3 * time.Second)
		res, err := h.CheckTx(ctx, txHash)
		if err != nil {
			continue
		}
		switch res.Status {
		case signing.TxStatusSucceeded:
			fmt.Printf("   verify ok: tx succeeded, gasUsed %s\n", res.GasUsed)
			return
		case signing.TxStatusFailed, signing.TxStatusDropped:
			log.Fatalf("   tx %s: %s", res.Status, res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}

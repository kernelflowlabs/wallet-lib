package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	tronrpc "github.com/kernelflowlabs/wallet-lib/rpc/tron"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/195'/0'/0/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://api.trongrid.io"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewTrxFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := tronrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	refInfo, err := h.InquireChain(ctx, "getRefInfo", "")
	if err != nil {
		log.Fatalf("getRefInfo: %v", err)
	}
	parts := strings.Split(refInfo, "_")
	if len(parts) != 3 {
		log.Fatalf("unexpected refInfo: %s", refInfo)
	}
	refBlockHash, refBlockNumber, refBlockTimestamp := parts[0], parts[1], parts[2]
	fmt.Printf("2. fetched from rpc: refBlockHash=%s refBlockNumber=%s refBlockTimestamp=%s\n", refBlockHash, refBlockNumber, refBlockTimestamp)

	b := tx.NewTrxTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNativeTRX).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetFeeLimit("0").
		SetRefBlockHash(refBlockHash).
		SetRefBlockNumber(refBlockNumber).
		SetRefBlockTime(refBlockTimestamp)
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
			fmt.Printf("   verify ok: tx succeeded at height %s, gasUsed %s\n", res.Height, res.GasUsed)
			return
		case signing.TxStatusFailed, signing.TxStatusDropped:
			log.Fatalf("   tx %s: %s", res.Status, res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}

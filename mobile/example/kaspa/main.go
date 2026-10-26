package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	kasrpc "github.com/kernelflowlabs/wallet-lib/rpc/kaspa"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/111111'/0'/0/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "grpc://n-mainnet.kaspa.ws:16110"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewKasFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "300000000"
	fee := "20000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := kasrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	utxoJSON, err := h.InquireChain(ctx, "getUtxo", sender)
	if err != nil {
		log.Fatalf("getUtxo: %v", err)
	}
	utxos := tx.NewMobileUtxoList()
	if err := utxos.SerializeFromStr(utxoJSON); err != nil {
		log.Fatalf("parse utxos: %v", err)
	}
	fmt.Printf("2. fetched from rpc: utxos=%s\n", utxos.String())

	b := tx.NewKasTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetFee(fee).
		SetUtxos(utxos)
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
			fmt.Printf("   verify ok: tx succeeded, height %s\n", res.Height)
			return
		case signing.TxStatusFailed, signing.TxStatusDropped:
			log.Fatalf("   tx %s: %s", res.Status, res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}

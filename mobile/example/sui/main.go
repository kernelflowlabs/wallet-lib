package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	suirpc "github.com/kernelflowlabs/wallet-lib/rpc/sui"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/784'/0'/0'/0'"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://fullnode.mainnet.sui.io:443"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewSuiFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1"
	gasBudget := "3000000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := suirpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	gasPrice, err := h.InquireChain(ctx, "getGasPrice", "")
	if err != nil {
		log.Fatalf("getGasPrice: %v", err)
	}
	coins, err := h.InquireChain(ctx, "getCoins", sender+"_"+signing.MagicContactAddressForNative)
	if err != nil {
		log.Fatalf("getCoins: %v", err)
	}
	fmt.Printf("2. fetched from rpc: gasPrice=%s coins=%s\n", gasPrice, coins)

	b := tx.NewSuiTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetGasPrice(gasPrice).
		SetGasBudget(gasBudget).
		SetCoins(coins)
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

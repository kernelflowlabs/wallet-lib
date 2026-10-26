package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	filecoinrpc "github.com/kernelflowlabs/wallet-lib/rpc/filecoin"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/461'/0'/0/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://api.node.glif.io/rpc/v0"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewFilecoinFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1000000000000000000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := filecoinrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	nonce, err := h.InquireChain(ctx, "getNonce", sender)
	if err != nil {
		nonce = "0"
		fmt.Printf("2. account not activated (%v); using nonce=0\n", err)
	}
	estParams, _ := json.Marshal(map[string]string{
		"from": sender, "to": recipient, "value": amount, "nonce": nonce,
	})
	gasStr, err := h.InquireChain(ctx, "estimateGas", string(estParams))
	if err != nil {
		log.Fatalf("estimateGas: %v", err)
	}
	var gas struct {
		GasLimit   string `json:"gasLimit"`
		GasFeeCap  string `json:"gasFeeCap"`
		GasPremium string `json:"gasPremium"`
	}
	if err := json.Unmarshal([]byte(gasStr), &gas); err != nil {
		log.Fatalf("parse gas: %v", err)
	}
	fmt.Printf("2. fetched from rpc: nonce=%s gasLimit=%s gasFeeCap=%s\n", nonce, gas.GasLimit, gas.GasFeeCap)

	b := tx.NewFilecoinTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetNonce(nonce).
		SetGasLimit(gas.GasLimit).
		SetGasFeeCap(gas.GasFeeCap).
		SetGasPremium(gas.GasPremium)
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
	fmt.Println("5. broadcasted, cid:", txHash)

	fmt.Println("6. waiting for confirmation...")
	for i := 0; i < 40; i++ {
		time.Sleep(3 * time.Second)
		res, err := h.CheckTx(ctx, txHash)
		if err != nil {
			continue
		}
		switch res.Status {
		case signing.TxStatusSucceeded:
			fmt.Printf("   verify ok: tx succeeded at height %s\n", res.Height)
			return
		case signing.TxStatusFailed:
			log.Fatalf("   tx failed: %s", res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}

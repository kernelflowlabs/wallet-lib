package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	mvxrpc "github.com/kernelflowlabs/wallet-lib/rpc/multiversx"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/multiversx"
)

const path = "m/44'/508'/0'/0'/0'"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://gateway.multiversx.com"
	network  = "1"
)

func main() {
	ctx := context.Background()
	fmt.Println("0. network:", network)

	a, err := acc.NewEgldFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := mvxrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	nonce, err := h.InquireChain(ctx, "getNonce", sender)
	if err != nil {
		log.Fatalf("getNonce: %v", err)
	}
	netCfgJSON, err := h.InquireChain(ctx, "getNetworkConfig", "")
	if err != nil {
		log.Fatalf("getNetworkConfig: %v", err)
	}
	var netCfg multiversx.NetWorkConfig
	if err := json.Unmarshal([]byte(netCfgJSON), &netCfg); err != nil {
		log.Fatalf("decode network config: %v", err)
	}
	fmt.Printf("2. fetched from rpc: nonce=%s chainID=%s gasPrice=%s gasLimit=%s version=%s\n",
		nonce, netCfg.ChainID, netCfg.GasPrice, netCfg.GasLimit, netCfg.Version)

	b := tx.NewEgldTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetNonce(nonce).
		SetChainId(netCfg.ChainID).
		SetGasPrice(netCfg.GasPrice).
		SetGasLimit(netCfg.GasLimit).
		SetVersion(netCfg.Version)
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
		res, err := h.CheckTx(ctx, txHash+":"+sender)
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

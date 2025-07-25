package cmd

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fatih/color"
)

// Clear performs the clear command
func Clear(rpcURL string, gasLimit uint64) error {
	if rpcURL == "" {
		rpcURL = DefaultRPCURL
	}

	// Explain why we need two private keys
	fmt.Println("We will need two private keys to clear the EIP-7702 authorization:")
	fmt.Println("")
	fmt.Println("1. The private key of the victim address that has been maliciously authorized.")
	fmt.Println("   This is required to sign the deauthorization transaction.")
	fmt.Println("")
	fmt.Println("2. The private key of a separate, secure address to pay for gas fees.")
	fmt.Println("   This is necessary because the victim address may not have funds to pay for")
	fmt.Println("   gas, or any funds sent to it might be immediately stolen by the attacker.")
	fmt.Println("")
	fmt.Println("The second address will only be used to broadcast the transaction and pay for gas.")
	fmt.Println("It should be a secure address with a small amount of ETH for transaction fees.")
	fmt.Println("")

	// Get victim private key
	color.Red("Please enter the private key of the address with malicious contract authorization:")
	victimPrivateKeyHex, err := readPrivateKey()
	if err != nil {
		return fmt.Errorf("error reading victim private key: %w", err)
	}

	victimPrivateKey, err := crypto.HexToECDSA(strings.TrimPrefix(victimPrivateKeyHex, "0x"))
	if err != nil {
		return fmt.Errorf("invalid victim private key: %w", err)
	}

	// Get relayer private key
	fmt.Println("\nPlease enter the private key of the address that will pay for gas fees:")
	relayerPrivateKeyHex, err := readPrivateKey()
	if err != nil {
		return fmt.Errorf("error reading relayer private key: %w", err)
	}

	relayerPrivateKey, err := crypto.HexToECDSA(strings.TrimPrefix(relayerPrivateKeyHex, "0x"))
	if err != nil {
		return fmt.Errorf("invalid relayer private key: %w", err)
	}

	// Get address from private key
	victimAddress := crypto.PubkeyToAddress(victimPrivateKey.PublicKey)
	relayerAddress := crypto.PubkeyToAddress(relayerPrivateKey.PublicKey)

	fmt.Printf("\nVictim address: %s\n", victimAddress.Hex())
	fmt.Printf("Relayer address: %s\n", relayerAddress.Hex())

	// Get chain ID
	chainID, err := getChainID(rpcURL)
	if err != nil {
		return fmt.Errorf("failed to get chain ID: %w", err)
	}
	fmt.Printf("\nChain ID: %d\n", chainID)

	// Get nonces
	victimNonce, err := getNonce(rpcURL, victimAddress.Hex())
	if err != nil {
		return fmt.Errorf("failed to get victim nonce: %w", err)
	}

	relayerNonce, err := getNonce(rpcURL, relayerAddress.Hex())
	if err != nil {
		return fmt.Errorf("failed to get relayer nonce: %w", err)
	}

	fmt.Printf("Victim nonce: %d\n", victimNonce)
	fmt.Printf("Relayer nonce: %d\n", relayerNonce)

	// Get gas parameters using EIP-1559 compatible method
	fmt.Println("\nFetching gas parameters from the network...")
	gasTip, gasFeeCap, err := getSuggestedGasFees(rpcURL)
	if err != nil {
		return fmt.Errorf("failed to get suggested gas fees: %w", err)
	}

	// Use the provided gas limit
	fmt.Printf("Using gas limit: %d\n", gasLimit)

	// Convert Wei to Gwei for display (1 Gwei = 10^9 Wei)
	weiToGwei := new(big.Float).SetFloat64(1000000000)

	gasTipGwei := new(big.Float).SetInt(gasTip)
	gasTipGwei.Quo(gasTipGwei, weiToGwei)

	gasFeeCapGwei := new(big.Float).SetInt(gasFeeCap)
	gasFeeCapGwei.Quo(gasFeeCapGwei, weiToGwei)

	// Calculate total max gas cost in ETH
	totalGasWei := new(big.Float).SetInt(gasFeeCap)
	totalGasWei.Mul(totalGasWei, new(big.Float).SetUint64(gasLimit))

	// 1 ETH = 10^18 Wei
	weiToEth := new(big.Float).SetFloat64(1000000000000000000)
	totalGasEth := new(big.Float).Set(totalGasWei)
	totalGasEth.Quo(totalGasEth, weiToEth)

	fmt.Printf("\nGas Information:\n")
	fmt.Printf("Max fee per gas: %.6f Gwei\n", gasFeeCapGwei)
	fmt.Printf("Priority fee: %.6f Gwei\n", gasTipGwei)
	fmt.Printf("Gas limit: %d\n", gasLimit)
	fmt.Printf("Estimated max gas cost: %.9f ETH\n", totalGasEth)

	// Confirm with user
	fmt.Println("\nAre you sure you want to clear the EIP-7702 authorization for this address? (y/n)")
	var confirmation string
	fmt.Scanln(&confirmation)
	if strings.ToLower(confirmation) != "y" && strings.ToLower(confirmation) != "yes" {
		return fmt.Errorf("operation cancelled by user")
	}

	// Create EIP-7702 authorization request
	req := SetAuthorizationRequest{
		UserEOAPrivateKey:    victimPrivateKey,
		UserEOANonce:         uint64(victimNonce),
		RelayerEOAPrivateKey: relayerPrivateKey,
		RelayerNonce:         uint64(relayerNonce),
		TemplateAddress:      common.Address{}, // Empty address to clear authorization
		ChainId:              chainID,
		GasTip:               gasTip,
		GasFeeCap:            gasFeeCap,
		GasLimit:             gasLimit,
	}

	fmt.Println("\nGenerating EIP-7702 deauthorization transaction...")
	signedTx, err := GenerateSet7702AuthTx(req)
	if err != nil {
		return fmt.Errorf("failed to generate transaction: %w", err)
	}

	fmt.Println("Broadcasting transaction...")
	txHash, err := broadcastRawTx(signedTx, rpcURL)
	if err != nil {
		return fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	color.Green("\nTransaction successfully sent!")
	color.Green("Transaction hash: %s", txHash)

	fmt.Println("\nWaiting for transaction to be mined...")
	// Wait for the transaction to be mined
	for i := 0; i < 60; i++ { // Try for 5 minutes (60 * 5 seconds)
		time.Sleep(5 * time.Second)
		receipt, err := getTransactionReceipt(rpcURL, txHash)
		if err == nil && receipt != nil {
			if receipt.Status == "0x1" {
				color.Green("\nTransaction successfully mined!")
				break
			} else if receipt.Status == "0x0" {
				return fmt.Errorf("transaction failed: %s", txHash)
			}
		}
		fmt.Print(".")
	}

	fmt.Println("\nTo verify the EIP-7702 authorization has been cleared, run:")
	fmt.Printf("eip7702cleaner check %s --rpc-url %s\n", victimAddress.Hex(), rpcURL)

	return nil
}

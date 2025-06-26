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

// Set performs the set command to authorize a specific contract address
func Set(contractAddress string, rpcURL string, gasLimit uint64) error {
	if rpcURL == "" {
		rpcURL = DefaultRPCURL
	}

	// Validate the contract address
	if !common.IsHexAddress(contractAddress) {
		return fmt.Errorf("invalid contract address format: %s", contractAddress)
	}

	templateAddress := common.HexToAddress(contractAddress)

	// Explain why we need two private keys
	fmt.Println("We will need two private keys to set the EIP-7702 authorization:")
	fmt.Println("")
	fmt.Println("1. The private key of the address that will be authorized to use the contract.")
	fmt.Println("   This is required to sign the authorization transaction.")
	fmt.Println("")
	fmt.Println("2. The private key of a separate address to pay for gas fees.")
	fmt.Println("   This address will broadcast the transaction and pay for gas.")
	fmt.Println("")
	fmt.Printf("The authorization will allow the first address to execute code from: %s\n", templateAddress.Hex())
	fmt.Println("")

	// Get user private key
	color.Yellow("Please enter the private key of the address to be authorized:")
	userPrivateKeyHex, err := readPrivateKey()
	if err != nil {
		return fmt.Errorf("error reading user private key: %w", err)
	}

	userPrivateKey, err := crypto.HexToECDSA(strings.TrimPrefix(userPrivateKeyHex, "0x"))
	if err != nil {
		return fmt.Errorf("invalid user private key: %w", err)
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

	// Get addresses from private keys
	userAddress := crypto.PubkeyToAddress(userPrivateKey.PublicKey)
	relayerAddress := crypto.PubkeyToAddress(relayerPrivateKey.PublicKey)

	fmt.Printf("\nUser address (to be authorized): %s\n", userAddress.Hex())
	fmt.Printf("Relayer address (pays gas): %s\n", relayerAddress.Hex())
	fmt.Printf("Contract address (to authorize): %s\n", templateAddress.Hex())

	// Get chain ID
	chainID, err := getChainID(rpcURL)
	if err != nil {
		return fmt.Errorf("failed to get chain ID: %w", err)
	}
	fmt.Printf("\nChain ID: %d\n", chainID)

	// Get nonces
	userNonce, err := getNonce(rpcURL, userAddress.Hex())
	if err != nil {
		return fmt.Errorf("failed to get user nonce: %w", err)
	}

	relayerNonce, err := getNonce(rpcURL, relayerAddress.Hex())
	if err != nil {
		return fmt.Errorf("failed to get relayer nonce: %w", err)
	}

	fmt.Printf("User nonce: %d\n", userNonce)
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
	color.Yellow("\nAre you sure you want to set the EIP-7702 authorization for this address? (y/n)")
	var confirmation string
	fmt.Scanln(&confirmation)
	if strings.ToLower(confirmation) != "y" && strings.ToLower(confirmation) != "yes" {
		return fmt.Errorf("operation cancelled by user")
	}

	// Create EIP-7702 authorization request
	req := SetAuthorizationRequest{
		UserEOAPrivateKey:    userPrivateKey,
		UserEOANonce:         uint64(userNonce),
		RelayerEOAPrivateKey: relayerPrivateKey,
		RelayerNonce:         uint64(relayerNonce),
		TemplateAddress:      templateAddress, // Set to specific contract address
		ChainId:              chainID,
		GasTip:               gasTip,
		GasFeeCap:            gasFeeCap,
		GasLimit:             gasLimit,
	}

	fmt.Println("\nGenerating EIP-7702 authorization transaction...")
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

	fmt.Println("\nTo verify the EIP-7702 authorization has been set, run:")
	fmt.Printf("eip7702cleaner check %s --rpc-url %s\n", userAddress.Hex(), rpcURL)

	return nil
}

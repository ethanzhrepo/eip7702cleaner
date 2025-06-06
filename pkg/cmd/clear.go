package cmd

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/fatih/color"
	"golang.org/x/term"
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

	// Get gas parameters
	gasPrice, err := getGasPrice(rpcURL)
	if err != nil {
		return fmt.Errorf("failed to get gas price: %w", err)
	}

	// Calculate gas parameters
	gasTip := new(big.Int).Div(gasPrice, big.NewInt(10))   // 10% of gas price
	gasFeeCap := new(big.Int).Mul(gasPrice, big.NewInt(2)) // 2x gas price for fee cap

	// Use the provided gas limit
	fmt.Printf("Using gas limit: %d\n", gasLimit)

	// Convert Wei to Gwei for display (1 Gwei = 10^9 Wei)
	weiToGwei := new(big.Float).SetFloat64(1000000000)

	gasPriceGwei := new(big.Float).SetInt(gasPrice)
	gasPriceGwei.Quo(gasPriceGwei, weiToGwei)

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
	fmt.Printf("Gas price: %.6f Gwei\n", gasPriceGwei)
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

// readPrivateKey reads a private key from stdin without echoing the input
func readPrivateKey() (string, error) {
	privateKeyBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	privateKey := strings.TrimSpace(string(privateKeyBytes))
	if privateKey == "" {
		return "", errors.New("private key cannot be empty")
	}
	return privateKey, nil
}

// getChainID gets the chain ID from the RPC endpoint
func getChainID(rpcURL string) (*big.Int, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_chainId",
		"params":  []interface{}{},
	}

	responseBody, err := makeRPCCall(rpcURL, body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Result string `json:"result"`
	}

	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, err
	}

	chainID := new(big.Int)
	chainID.SetString(result.Result[2:], 16) // Remove "0x" prefix and parse as hex

	return chainID, nil
}

// getNonce gets the nonce for an address
func getNonce(rpcURL, address string) (int64, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getTransactionCount",
		"params":  []interface{}{address, "latest"},
	}

	responseBody, err := makeRPCCall(rpcURL, body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Result string `json:"result"`
	}

	if err := json.Unmarshal(responseBody, &result); err != nil {
		return 0, err
	}

	nonce := new(big.Int)
	nonce.SetString(result.Result[2:], 16) // Remove "0x" prefix and parse as hex

	return nonce.Int64(), nil
}

// getGasPrice gets the current gas price
func getGasPrice(rpcURL string) (*big.Int, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_gasPrice",
		"params":  []interface{}{},
	}

	responseBody, err := makeRPCCall(rpcURL, body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Result string `json:"result"`
	}

	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, err
	}

	gasPrice := new(big.Int)
	gasPrice.SetString(result.Result[2:], 16) // Remove "0x" prefix and parse as hex

	return gasPrice, nil
}

// getTransactionReceipt gets the receipt for a transaction
func getTransactionReceipt(rpcURL, txHash string) (*TransactionReceipt, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getTransactionReceipt",
		"params":  []interface{}{txHash},
	}

	responseBody, err := makeRPCCall(rpcURL, body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Result *TransactionReceipt `json:"result"`
	}

	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, err
	}

	return result.Result, nil
}

// TransactionReceipt represents the structure of an Ethereum transaction receipt
type TransactionReceipt struct {
	TransactionHash   string `json:"transactionHash"`
	TransactionIndex  string `json:"transactionIndex"`
	BlockHash         string `json:"blockHash"`
	BlockNumber       string `json:"blockNumber"`
	Status            string `json:"status"`
	GasUsed           string `json:"gasUsed"`
	CumulativeGasUsed string `json:"cumulativeGasUsed"`
}

// makeRPCCall is a helper function to make RPC calls
func makeRPCCall(rpcURL string, body map[string]interface{}) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return responseBody, nil
}

const (
	SET_CODE_TX_TYPE = 0x04
	MAGIC            = 0x05
)

// 计算授权元组的签名消息
func authTupleMessage(chainId *big.Int, addr common.Address, nonce uint64) []byte {
	var buf bytes.Buffer
	rlp.Encode(&buf, []interface{}{chainId, addr, nonce})
	msg := append([]byte{MAGIC}, buf.Bytes()...)
	return crypto.Keccak256(msg)
}

func build7702Tx(
	chainId *big.Int,
	userPriv *ecdsa.PrivateKey,
	relayerNonce uint64,
	userNonce uint64,
	gasTip *big.Int,
	gasFeeCap *big.Int,
	gasLimit uint64,
	contractAddr common.Address,
	txData []byte,
) (string, error) {

	authMsg := authTupleMessage(chainId, contractAddr, userNonce)
	sig, err := crypto.Sign(authMsg, userPriv)
	if err != nil {
		return "", err
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	yParity := uint8(sig[64])

	rawTx := []interface{}{
		chainId, relayerNonce, gasTip, gasFeeCap, gasLimit, contractAddr, big.NewInt(0), txData,
		[]interface{}{}, // access_list
		[]interface{}{
			[]interface{}{chainId, contractAddr, userNonce, yParity, r, s},
		},
	}
	rlpPayload, err := rlp.EncodeToBytes(rawTx)
	if err != nil {
		return "", err
	}
	finalTx := append([]byte{SET_CODE_TX_TYPE}, rlpPayload...)
	return hex.EncodeToString(finalTx), nil
}

// CallTuple defines the parameters for each batched asset collection call.
type CallTuple struct {
	To    common.Address
	Value *big.Int
	Data  []byte
}

// SetAuthorizationRequest holds the request parameters for EIP-7702 authorization.
type SetAuthorizationRequest struct {
	UserEOAPrivateKey    *ecdsa.PrivateKey
	UserEOANonce         uint64
	RelayerEOAPrivateKey *ecdsa.PrivateKey
	RelayerNonce         uint64
	TemplateAddress      common.Address
	ChainId              *big.Int
	GasTip               *big.Int // Optional, will use suggestion if nil
	GasFeeCap            *big.Int // Optional, will use suggestion if nil
	GasLimit             uint64   // Optional, will use suggestion if 0
}

// GenerateSet7702AuthTx generates an EIP-7702 authorization transaction.
// Returns a hex string of the signed transaction ready for broadcast.
func GenerateSet7702AuthTx(req SetAuthorizationRequest) (string, error) {
	unsignedTxHex, err := build7702Tx(
		req.ChainId,
		req.UserEOAPrivateKey,
		req.RelayerNonce,
		req.UserEOANonce,
		req.GasTip,
		req.GasFeeCap,
		req.GasLimit,
		common.Address{},
		[]byte{},
	)
	if err != nil {
		return "", err
	}

	signedHex, err := signEIP7702Tx(unsignedTxHex, req.RelayerEOAPrivateKey)
	if err != nil {
		return "", err
	}
	return signedHex, nil
}

func signEIP7702Tx(rawHex string, relayerPriv *ecdsa.PrivateKey) (string, error) {
	txBytes, err := hex.DecodeString(rawHex)
	if err != nil {
		return "", err
	}
	if len(txBytes) < 1 || txBytes[0] != 0x04 {
		return "", errors.New("not a EIP-7702 tx hex")
	}
	payload := txBytes[1:]

	var txRaw []interface{}
	if err := rlp.DecodeBytes(payload, &txRaw); err != nil {
		return "", err
	}
	hash := crypto.Keccak256(append([]byte{0x04}, payload...))
	sig, err := crypto.Sign(hash, relayerPriv)
	if err != nil {
		return "", err
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	yParity := uint8(sig[64])

	txRaw = append(txRaw, yParity, r, s)
	finalPayload, err := rlp.EncodeToBytes(txRaw)
	if err != nil {
		return "", err
	}
	finalTx := append([]byte{0x04}, finalPayload...)
	return hex.EncodeToString(finalTx), nil
}

func broadcastRawTx(rawTxHex string, rpcUrl string) (string, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_sendRawTransaction",
		"params":  []string{"0x" + rawTxHex},
	}
	payload, _ := json.Marshal(body)
	resp, err := http.Post(rpcUrl, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bz, _ := io.ReadAll(resp.Body)
	var result struct {
		Result string `json:"result"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(bz, &result)
	if result.Error.Message != "" {
		return "", errors.New(result.Error.Message)
	}
	return result.Result, nil
}

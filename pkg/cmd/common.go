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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/term"
)

// DefaultRPCURL is the default RPC URL if not specified
const DefaultRPCURL = "https://ethereum-rpc.publicnode.com"

// EIP-7702 constants
const (
	SET_CODE_TX_TYPE = 0x04
	MAGIC            = 0x05
)

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

// getSuggestedGasFees queries the RPC for EIP-1559 gas fee suggestions.
// It returns maxPriorityFeePerGas and maxFeePerGas.
func getSuggestedGasFees(rpcURL string) (*big.Int, *big.Int, error) {
	// 1. Try to get maxPriorityFeePerGas (the "tip")
	priorityFeeBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_maxPriorityFeePerGas",
		"params":  []interface{}{},
	}

	respBody, err := makeRPCCall(rpcURL, priorityFeeBody)
	if err != nil {
		// Fallback for networks that don't support eth_maxPriorityFeePerGas
		return fallbackGasFees(rpcURL)
	}

	var priorityFeeResult struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(respBody, &priorityFeeResult); err != nil || priorityFeeResult.Result == "" {
		// Fallback for networks that don't support eth_maxPriorityFeePerGas
		return fallbackGasFees(rpcURL)
	}

	maxPriorityFeePerGas := new(big.Int)
	maxPriorityFeePerGas.SetString(strings.TrimPrefix(priorityFeeResult.Result, "0x"), 16)

	// 2. Get the latest block to find baseFeePerGas
	blockBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getBlockByNumber",
		"params":  []interface{}{"latest", false},
	}

	respBody, err = makeRPCCall(rpcURL, blockBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get latest block: %w", err)
	}

	var blockResult struct {
		Result struct {
			BaseFeePerGas string `json:"baseFeePerGas"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &blockResult); err != nil || blockResult.Result.BaseFeePerGas == "" {
		// Some networks might not have baseFeePerGas, use legacy calculation
		return fallbackGasFees(rpcURL)
	}

	baseFeePerGas := new(big.Int)
	baseFeePerGas.SetString(strings.TrimPrefix(blockResult.Result.BaseFeePerGas, "0x"), 16)

	// 3. Calculate maxFeePerGas
	// A common strategy: maxFeePerGas = (2 * baseFee) + maxPriorityFee
	gasFeeCap := new(big.Int).Add(
		new(big.Int).Mul(baseFeePerGas, big.NewInt(2)),
		maxPriorityFeePerGas,
	)

	// Ensure minimum fees for networks like BSC
	minPriorityFee := big.NewInt(100000000) // 0.1 Gwei minimum
	if maxPriorityFeePerGas.Cmp(minPriorityFee) < 0 {
		maxPriorityFeePerGas = minPriorityFee
		// Recalculate gasFeeCap with the minimum priority fee
		gasFeeCap = new(big.Int).Add(
			new(big.Int).Mul(baseFeePerGas, big.NewInt(2)),
			maxPriorityFeePerGas,
		)
	}

	return maxPriorityFeePerGas, gasFeeCap, nil
}

// fallbackGasFees provides a fallback method for networks that don't support EIP-1559
func fallbackGasFees(rpcURL string) (*big.Int, *big.Int, error) {
	gasPrice, err := getGasPrice(rpcURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get gas price for fallback: %w", err)
	}

	// For networks without EIP-1559, use the same value for both
	// But ensure minimum priority fee for networks like BSC
	minPriorityFee := big.NewInt(100000000) // 0.1 Gwei minimum

	if gasPrice.Cmp(minPriorityFee) < 0 {
		gasPrice = minPriorityFee
	}

	// In fallback mode, tip and fee cap are the same
	return gasPrice, gasPrice, nil
}

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
		req.TemplateAddress,
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

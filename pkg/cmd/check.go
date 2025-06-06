package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/fatih/color"
)

// Version holds the current version of the application
// This will be set at build time via LDFLAGS in the Makefile
var Version = "dev"

// DefaultRPCURL is the default RPC URL if not specified
const DefaultRPCURL = "https://ethereum-rpc.publicnode.com"

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  string      `json:"result"`
	Error   interface{} `json:"error,omitempty"`
}

// Check performs the check command
func Check(address string, rpcURL string, debug bool) error {
	// debug = true

	if debug {
		fmt.Println("========== DEBUG INFO START ==========")
		fmt.Printf("Check function called with address: %s\n", address)
		fmt.Printf("RPC URL parameter: '%s'\n", rpcURL)
	}

	if address == "" {
		return fmt.Errorf("address is required")
	}

	// Fix: rpcURL might be empty even when passed from command line
	if rpcURL == "" {
		rpcURL = DefaultRPCURL
		if debug {
			fmt.Printf("Using default RPC URL: %s\n", rpcURL)
		}
	} else {
		if debug {
			fmt.Printf("Using provided RPC URL: %s\n", rpcURL)
		}
	}

	// Debug information
	if debug {
		fmt.Printf("Debug - Using RPC URL: %s\n", rpcURL)
		fmt.Printf("Debug - Checking address: %s\n", address)
	}

	// Validate Ethereum address
	if !common.IsHexAddress(address) {
		return fmt.Errorf("invalid Ethereum address format: %s", address)
	}

	// Convert to checksum address
	checksumAddr := common.HexToAddress(address)
	if debug {
		fmt.Printf("Debug - Checksum address: %s\n", checksumAddr.Hex())
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create JSON-RPC request
	request := RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_getCode",
		Params:  []interface{}{checksumAddr.Hex(), "latest"},
		ID:      1,
	}

	// Marshal request to JSON
	requestJSON, err := json.Marshal(request)
	if err != nil {
		if debug {
			fmt.Printf("Error marshaling request: %v\n", err)
		}
		return fmt.Errorf("failed to marshal JSON-RPC request: %w", err)
	}

	if debug {
		fmt.Printf("Debug - JSON-RPC Request: %s\n", string(requestJSON))
		fmt.Printf("Sending HTTP request to: %s\n", rpcURL)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", rpcURL, bytes.NewBuffer(requestJSON))
	if err != nil {
		if debug {
			fmt.Printf("Error creating HTTP request: %v\n", err)
		}
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	if debug {
		fmt.Println("Sending HTTP request...")
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		if debug {
			fmt.Printf("HTTP request failed: %v\n", err)
		}
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if debug {
			fmt.Printf("Error reading response body: %v\n", err)
		}
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if debug {
		fmt.Printf("Debug - HTTP Status: %d\n", resp.StatusCode)
		fmt.Printf("Debug - Raw HTTP Response: %s\n", string(body))
	}

	// Parse JSON-RPC response
	var rpcResponse RPCResponse
	err = json.Unmarshal(body, &rpcResponse)
	if err != nil {
		if debug {
			fmt.Printf("Error unmarshaling response: %v\n", err)
		}
		return fmt.Errorf("failed to unmarshal JSON-RPC response: %w", err)
	}

	// Check for RPC error
	if rpcResponse.Error != nil {
		if debug {
			fmt.Printf("RPC Error: %v\n", rpcResponse.Error)
		}
		return fmt.Errorf("JSON-RPC error: %v", rpcResponse.Error)
	}

	// Store the result
	result := rpcResponse.Result

	if debug {
		fmt.Printf("Debug - RPC Result: %s\n", result)
		fmt.Println("========== DEBUG INFO END ==========")
	}

	// If no code is found or only "0x", the address is safe (not a contract)
	if result == "" || result == "0x" {
		if debug {
			fmt.Printf("Debug - No code found, considering address safe\n")
		}
		color.Green("✓ Address %s is safe (no code detected)", address)
		return nil
	}

	// Remove "0x" prefix if present for processing
	codeWithoutPrefix := result
	if strings.HasPrefix(result, "0x") {
		codeWithoutPrefix = result[2:]
	}

	// Convert to lowercase for matching
	codeHexLower := strings.ToLower(codeWithoutPrefix)

	if debug {
		fmt.Printf("Debug - Code after 0x removal: %s\n", codeWithoutPrefix)
		fmt.Printf("Debug - Code lowercase: %s\n", codeHexLower)
		fmt.Printf("Debug - Checking if starts with ef0100: %v\n", strings.HasPrefix(codeHexLower, "ef0100"))
	}

	// Check if the code starts with ef0100
	if strings.HasPrefix(codeHexLower, "ef0100") {
		// Extract the contract address (remove ef0100 prefix and add 0x)
		contractAddr := "0x" + codeWithoutPrefix[6:]
		if debug {
			fmt.Printf("Debug - Extracted contract address: %s\n", contractAddr)
		}
		color.Red("⚠ Address %s has an EIP-7702 contract deployed", address)
		color.Red("⚠ Contract address: %s", contractAddr)
		return nil
	}

	// Code exists but doesn't match EIP-7702 pattern
	if debug {
		fmt.Printf("Debug - Code exists but does not match EIP-7702 pattern\n")
	}
	color.Yellow("⚠ Address %s has code deployed and might be a contract", address)
	return nil
}

package main

import (
	"fmt"
	"os"
	"os/signal"

	cmdpkg "github.com/ethanzhrepo/eip7702cleaner/pkg/cmd"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	// 命令行标志
	rpcURL   string
	debug    bool
	gasLimit uint64

	// 根命令
	rootCmd = &cobra.Command{
		Use:   "eip7702cleaner",
		Short: "EIP-7702 Cleaner Tool",
		Long:  `A command-line tool for checking and cleaning EIP-7702 contracts on Ethereum addresses.`,
	}

	// check 子命令
	checkCmd = &cobra.Command{
		Use:   "check [address]",
		Short: "Check if an address has an EIP-7702 contract",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			address := args[0]

			// 仅在debug模式下显示解析信息
			if debug {
				fmt.Printf("Debug - Cobra parsing - Address: %s\n", address)
				fmt.Printf("Debug - Cobra parsing - RPC URL: %s\n", rpcURL)
				fmt.Printf("Debug - Cobra parsing - Debug: %v\n", debug)
				fmt.Printf("Debug - Cobra parsing - Gas Limit: %d\n", gasLimit)
			}

			err := cmdpkg.Check(address, rpcURL, debug)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	// clear 子命令
	clearCmd = &cobra.Command{
		Use:   "clear",
		Short: "Clear an EIP-7702 contract from an address",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			// 仅在debug模式下显示解析信息
			if debug {
				fmt.Printf("Debug - Cobra parsing - RPC URL: %s\n", rpcURL)
				fmt.Printf("Debug - Cobra parsing - Debug: %v\n", debug)
				fmt.Printf("Debug - Cobra parsing - Gas Limit: %d\n", gasLimit)
			}

			err := cmdpkg.Clear(rpcURL, gasLimit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}
)

func init() {
	// 为子命令添加标志
	checkCmd.Flags().StringVar(&rpcURL, "rpc-url", "", "RPC URL for Ethereum node")
	checkCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug output")

	clearCmd.Flags().StringVar(&rpcURL, "rpc-url", "", "RPC URL for Ethereum node")
	clearCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug output")

	// 添加 gas-limit 标志
	rootCmd.PersistentFlags().Uint64Var(&gasLimit, "gas-limit", 100000, "Gas limit for transactions")

	// 添加子命令到根命令
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(clearCmd)
}

func main() {
	fd := int(os.Stdin.Fd())

	oldState, err := term.GetState(fd)
	if err != nil {
		fmt.Printf("\nError getting terminal state: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(fd, oldState)

	// 监听ctrl+c, 恢复终端状态
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		term.Restore(fd, oldState)
		fmt.Println("Ctrl+C pressed, exiting...")
		os.Exit(0)
	}()

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

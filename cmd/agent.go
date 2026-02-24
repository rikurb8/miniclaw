/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"

	"miniclaw/pkg/config"

	"github.com/spf13/cobra"
)

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Load config and start the agent",
	Long: `Loads MiniClaw configuration from config.json and prints it,
then confirms the agent command is ready to run.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("failed to load config: %v\n", err)
			return
		}

		encoded, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Printf("failed to print config: %v\n", err)
			return
		}

		fmt.Println(string(encoded))
		fmt.Println("🦞 agent ready")
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
}

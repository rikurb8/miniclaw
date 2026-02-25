package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"miniclaw/pkg/channel"
	"miniclaw/pkg/channel/telegram"
	"miniclaw/pkg/config"
	"miniclaw/pkg/gateway"
	"miniclaw/pkg/logger"

	"github.com/spf13/cobra"
)

const telegramChannelName = "telegram"

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Run channel gateway mode",
	Long:  "Runs MiniClaw as a channel gateway with health and readiness endpoints.",
	Run: func(cmd *cobra.Command, args []string) {
		_ = args

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("failed to load config: %v\n", err)
			return
		}

		appLogger, err := logger.New(cfg.Logging)
		if err != nil {
			fmt.Printf("failed to initialize logger: %v\n", err)
			return
		}
		slog.SetDefault(appLogger)
		log := slog.Default().With("component", "cmd.gateway")

		adapters, err := enabledAdapters(cfg, log)
		if err != nil {
			log.Error("Gateway configuration invalid", "error", err)
			return
		}

		runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		svc, err := gateway.NewService(runCtx, cfg, adapters, log)
		if err != nil {
			log.Error("Failed to initialize gateway service", "error", err)
			return
		}

		log.Info("Gateway started", "channels", enabledChannelNames(adapters), "provider", cfg.Agents.Defaults.Provider, "model", cfg.Agents.Defaults.Model)
		if err := svc.Run(runCtx); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Error("Gateway runtime failed", "error", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(gatewayCmd)
}

func enabledAdapters(cfg *config.Config, log *slog.Logger) ([]channel.Adapter, error) {
	adapters := make([]channel.Adapter, 0, 1)

	if cfg.Channels.Telegram.Enabled {
		adapter, err := telegram.NewAdapter(cfg.Channels.Telegram, log)
		if err != nil {
			return nil, fmt.Errorf("configure %s channel: %w", telegramChannelName, err)
		}
		adapters = append(adapters, adapter)
	}

	if len(adapters) == 0 {
		return nil, errors.New("no channels are enabled")
	}

	return adapters, nil
}

func enabledChannelNames(adapters []channel.Adapter) string {
	names := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		names = append(names, adapter.Name())
	}

	return strings.Join(names, ",")
}

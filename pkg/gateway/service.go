package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	agentruntime "miniclaw/pkg/agent/runtime"
	"miniclaw/pkg/bus"
	"miniclaw/pkg/channel"
	"miniclaw/pkg/config"
	"miniclaw/pkg/provider"
)

const (
	defaultHealthHost = "0.0.0.0"
	defaultHealthPort = 18790
)

type Service struct {
	cfg      *config.Config
	log      *slog.Logger
	provider provider.Client
	manager  *runtimeManager
	channels []channel.Adapter

	mu               sync.RWMutex
	startedAt        time.Time
	providerLastOKAt time.Time
	providerLastErr  string
	channelStates    map[string]channelState
}

type channelState struct {
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

type statusResponse struct {
	Status           string                  `json:"status"`
	UptimeSeconds    int64                   `json:"uptime_seconds"`
	ProviderLastOKAt string                  `json:"provider_last_ok_at,omitempty"`
	ProviderLastErr  string                  `json:"provider_last_error,omitempty"`
	Channels         map[string]channelState `json:"channels"`
}

func NewService(ctx context.Context, cfg *config.Config, adapters []channel.Adapter, log *slog.Logger) (*Service, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if len(adapters) == 0 {
		return nil, errors.New("at least one channel adapter is required")
	}
	if log == nil {
		log = slog.Default()
	}

	client, err := provider.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize provider: %w", err)
	}

	manager, err := newRuntimeManager(ctx, cfg, client, log)
	if err != nil {
		return nil, err
	}

	channelStates := make(map[string]channelState, len(adapters))
	for _, adapter := range adapters {
		channelStates[adapter.Name()] = channelState{}
	}

	return &Service{
		cfg:           cfg,
		log:           log.With("component", "gateway.service"),
		provider:      client,
		manager:       manager,
		channels:      adapters,
		channelStates: channelStates,
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	s.startedAt = time.Now().UTC()
	s.mu.Unlock()

	if err := s.checkProviderHealth(ctx); err != nil {
		return err
	}

	serverErrors := make(chan error, 1)
	go s.runHealthServer(ctx, serverErrors)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.checkProviderHealth(ctx)
			}
		}
	}()

	errCh := make(chan error, len(s.channels))
	for _, adapter := range s.channels {
		adapter := adapter
		s.setChannelState(adapter.Name(), channelState{Running: true})

		go func() {
			err := adapter.Run(ctx, s.handleInbound)
			s.setChannelState(adapter.Name(), channelState{Running: false, Error: errorString(err)})
			if err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("run %s channel: %w", adapter.Name(), err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		s.manager.Close()
		return nil
	case err := <-serverErrors:
		s.manager.Close()
		return err
	case err := <-errCh:
		s.manager.Close()
		return err
	}
}

func (s *Service) handleInbound(ctx context.Context, inbound bus.InboundMessage) (bus.OutboundMessage, error) {
	result, err := s.manager.Prompt(ctx, inbound.SessionKey, inbound.Content)
	if err != nil {
		return bus.OutboundMessage{
			Channel:    inbound.Channel,
			ChatID:     inbound.ChatID,
			SessionKey: inbound.SessionKey,
			Error:      err.Error(),
		}, err
	}

	return bus.OutboundMessage{
		Channel:    inbound.Channel,
		ChatID:     inbound.ChatID,
		SessionKey: inbound.SessionKey,
		Content:    result.Text,
		Metadata:   agentruntime.PromptResultMetadata(result),
	}, nil
}

func (s *Service) runHealthServer(ctx context.Context, errCh chan<- error) {
	host := strings.TrimSpace(s.cfg.Gateway.Host)
	if host == "" {
		host = defaultHealthHost
	}

	port := s.cfg.Gateway.Port
	if port <= 0 {
		port = defaultHealthPort
	}

	addr := host + ":" + strconv.Itoa(port)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	s.log.Info("Gateway status server started", "address", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errCh <- fmt.Errorf("start status server: %w", err)
	}
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.respondStatus(w, http.StatusOK, "ok")
}

func (s *Service) handleReady(w http.ResponseWriter, _ *http.Request) {
	statusCode := http.StatusOK
	status := "ready"
	if !s.isReady() {
		statusCode = http.StatusServiceUnavailable
		status = "not_ready"
	}

	s.respondStatus(w, statusCode, status)
}

func (s *Service) respondStatus(w http.ResponseWriter, statusCode int, status string) {
	payload := s.currentStatus(status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.log.Error("Failed to write status response", "error", err)
	}
}

func (s *Service) currentStatus(status string) statusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uptime := int64(0)
	if !s.startedAt.IsZero() {
		uptime = int64(time.Since(s.startedAt).Seconds())
	}

	channels := make(map[string]channelState, len(s.channelStates))
	for name, state := range s.channelStates {
		channels[name] = state
	}

	providerLastOK := ""
	if !s.providerLastOKAt.IsZero() {
		providerLastOK = s.providerLastOKAt.Format(time.RFC3339)
	}

	return statusResponse{
		Status:           status,
		UptimeSeconds:    uptime,
		ProviderLastOKAt: providerLastOK,
		ProviderLastErr:  s.providerLastErr,
		Channels:         channels,
	}
}

func (s *Service) isReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.channelStates) == 0 {
		return false
	}

	anyRunning := false
	for _, state := range s.channelStates {
		if state.Running {
			anyRunning = true
			break
		}
	}

	if !anyRunning {
		return false
	}

	if s.providerLastOKAt.IsZero() {
		return false
	}

	if s.providerLastErr != "" {
		return false
	}

	return true
}

func (s *Service) checkProviderHealth(ctx context.Context) error {
	if err := s.provider.Health(ctx); err != nil {
		s.mu.Lock()
		s.providerLastErr = err.Error()
		s.mu.Unlock()
		return fmt.Errorf("provider health check failed: %w", err)
	}

	s.mu.Lock()
	s.providerLastErr = ""
	s.providerLastOKAt = time.Now().UTC()
	s.mu.Unlock()

	return nil
}

func (s *Service) setChannelState(name string, state channelState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelStates[name] = state
}

func errorString(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

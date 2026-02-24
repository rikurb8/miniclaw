package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	charmLog "github.com/charmbracelet/log"

	"miniclaw/pkg/config"
)

const (
	defaultFormat = "text"
	defaultLevel  = "info"
)

type LogEntry struct {
	Level     string         `json:"level"`
	Timestamp string         `json:"timestamp"`
	Component string         `json:"component,omitempty"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Caller    string         `json:"caller,omitempty"`
}

type entryHandler struct {
	level     slog.Level
	addSource bool
	writer    io.Writer
	attrs     []slog.Attr
	groups    []string
	mu        *sync.Mutex
}

func New(cfg config.LoggingConfig) (*slog.Logger, error) {
	return newWithWriter(cfg, os.Stderr)
}

func newWithWriter(cfg config.LoggingConfig, writer io.Writer) (*slog.Logger, error) {
	format := strings.ToLower(strings.TrimSpace(cfg.Format))
	if value := strings.TrimSpace(os.Getenv("MINICLAW_LOG_FORMAT")); value != "" {
		format = strings.ToLower(value)
	}
	if format == "" {
		format = defaultFormat
	}
	if format != "json" && format != "text" {
		return nil, fmt.Errorf("unsupported log format %q", format)
	}

	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	addSource := cfg.AddSource
	if env := strings.TrimSpace(os.Getenv("MINICLAW_LOG_ADD_SOURCE")); env != "" {
		addSource = parseBool(env)
	}

	h := &entryHandler{
		level:     level,
		addSource: addSource,
		writer:    writer,
		mu:        &sync.Mutex{},
	}

	if format == "text" {
		pretty := charmLog.NewWithOptions(writer, charmLog.Options{
			Level:           charmLevel(level),
			ReportTimestamp: true,
			ReportCaller:    addSource,
			Formatter:       charmLog.TextFormatter,
		})
		return slog.New(pretty), nil
	}

	return slog.New(h), nil
}

func charmLevel(level slog.Level) charmLog.Level {
	switch {
	case level <= slog.LevelDebug:
		return charmLog.DebugLevel
	case level <= slog.LevelInfo:
		return charmLog.InfoLevel
	case level <= slog.LevelWarn:
		return charmLog.WarnLevel
	default:
		return charmLog.ErrorLevel
	}
}

func parseLevel(input string) (slog.Level, error) {
	levelText := strings.ToLower(strings.TrimSpace(input))
	if value := strings.TrimSpace(os.Getenv("MINICLAW_LOG_LEVEL")); value != "" {
		levelText = strings.ToLower(value)
	}
	if levelText == "" {
		levelText = defaultLevel
	}

	switch levelText {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", levelText)
	}
}

func parseBool(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (h *entryHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *entryHandler) Handle(_ context.Context, record slog.Record) error {
	entry := LogEntry{
		Level:     strings.ToLower(record.Level.String()),
		Timestamp: record.Time.UTC().Format(time.RFC3339Nano),
		Message:   record.Message,
	}
	if record.Time.IsZero() {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	fields := make(map[string]any)

	for _, attr := range h.attrs {
		applyAttr(fields, &entry, h.groups, attr)
	}

	record.Attrs(func(attr slog.Attr) bool {
		applyAttr(fields, &entry, h.groups, attr)
		return true
	})

	if len(fields) > 0 {
		entry.Fields = fields
	}

	if h.addSource {
		entry.Caller = callerFromRecord(record)
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.writer.Write(append(line, '\n'))
	return err
}

func callerFromRecord(record slog.Record) string {
	if record.PC == 0 {
		return ""
	}

	frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
	if frame.File == "" {
		return ""
	}

	return fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
}

func applyAttr(fields map[string]any, entry *LogEntry, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string{}, groups...), attr.Key), ".")
	}

	if key == "component" {
		if value, ok := attr.Value.Any().(string); ok {
			entry.Component = value
			return
		}
	}

	fields[key] = attrValue(attr.Value)
}

func attrValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().UTC().Format(time.RFC3339Nano)
	case slog.KindGroup:
		group := value.Group()
		result := make(map[string]any, len(group))
		for _, item := range group {
			result[item.Key] = attrValue(item.Value.Resolve())
		}
		return result
	case slog.KindAny:
		return value.Any()
	default:
		return value.String()
	}
}

func (h *entryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

func (h *entryHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.groups = append(append([]string{}, h.groups...), name)
	return &next
}

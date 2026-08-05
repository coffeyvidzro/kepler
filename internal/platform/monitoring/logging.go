package monitoring

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
)

type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

type LoggingConfig struct {
	Level     string
	Format    LogFormat
	AddSource bool
	Output    io.Writer
}

// ConfigureLogging creates and installs the process-wide structured logger.
func ConfigureLogging(configuration LoggingConfig) (*slog.Logger, error) {
	level, err := parseLogLevel(configuration.Level)
	if err != nil {
		return nil, err
	}
	output := configuration.Output
	if output == nil {
		output = os.Stdout
	}
	options := &slog.HandlerOptions{Level: level, AddSource: configuration.AddSource}

	var handler slog.Handler
	switch configuration.Format {
	case "", LogFormatJSON:
		handler = slog.NewJSONHandler(output, options)
	case LogFormatText:
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, errors.New("log format must be json or text")
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, errors.New("log level must be debug, info, warn, or error")
	}
}

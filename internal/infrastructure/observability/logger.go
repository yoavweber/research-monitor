package observability

import (
	"context"
	"log/slog"
	"os"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

type slogLogger struct{ l *slog.Logger }

func NewLogger(appEnv string) shared.Logger {
	var handler slog.Handler
	if appEnv == "prod" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return &slogLogger{l: slog.New(handler)}
}

func (s *slogLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	s.l.InfoContext(ctx, msg, args...)
}
func (s *slogLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	s.l.WarnContext(ctx, msg, args...)
}
func (s *slogLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	s.l.ErrorContext(ctx, msg, args...)
}
func (s *slogLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	s.l.DebugContext(ctx, msg, args...)
}
func (s *slogLogger) With(args ...any) shared.Logger {
	return &slogLogger{l: s.l.With(args...)}
}

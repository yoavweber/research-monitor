package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// LogRecord captures a single structured-log emission so tests can assert
// on (Level, Msg, Args) without parsing slog handler output.
type LogRecord struct {
	Level string
	Msg   string
	Args  map[string]any
}

// RecordingLogger is a shared.Logger fake that records every call into an
// in-memory slice. Zero value is ready to use; concurrent calls are guarded
// by a mutex.
type RecordingLogger struct {
	mu      sync.Mutex
	Records []LogRecord
}

func (l *RecordingLogger) record(level, msg string, args []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		m[k] = args[i+1]
	}
	l.Records = append(l.Records, LogRecord{Level: level, Msg: msg, Args: m})
}

func (l *RecordingLogger) InfoContext(_ context.Context, msg string, args ...any) {
	l.record("Info", msg, args)
}
func (l *RecordingLogger) WarnContext(_ context.Context, msg string, args ...any) {
	l.record("Warn", msg, args)
}
func (l *RecordingLogger) ErrorContext(_ context.Context, msg string, args ...any) {
	l.record("Error", msg, args)
}
func (l *RecordingLogger) DebugContext(_ context.Context, msg string, args ...any) {
	l.record("Debug", msg, args)
}

// With is a no-op: assertions key off the recorded args from each call site,
// not off accumulated context.
func (l *RecordingLogger) With(_ ...any) shared.Logger { return l }

// RecordsAt returns every recorded entry at the given level, in call order.
func (l *RecordingLogger) RecordsAt(level string) []LogRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []LogRecord
	for _, r := range l.Records {
		if r.Level == level {
			out = append(out, r)
		}
	}
	return out
}

package logging

import (
	"context"
	"log/slog"
)

// RedactingHandler wraps an slog.Handler to scrub sensitive fields before logging.
type RedactingHandler struct {
	inner         slog.Handler
	attrs         []slog.Attr
	sensitiveKeys map[string]bool
}

// NewRedactingHandler wraps an existing handler with field redaction.
// Any log attribute whose key is in sensitiveKeys will have its value replaced.
func NewRedactingHandler(inner slog.Handler, sensitiveKeys []string) *RedactingHandler {
	keys := make(map[string]bool, len(sensitiveKeys))
	for _, k := range sensitiveKeys {
		keys[k] = true
	}
	return &RedactingHandler{inner: inner, sensitiveKeys: keys}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	redacted := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	for _, a := range h.attrs {
		redacted.AddAttrs(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		redacted.AddAttrs(h.redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, redacted)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = h.redactAttr(a)
	}
	return &RedactingHandler{
		inner:         h.inner,
		attrs:         append(h.attrs, redacted...),
		sensitiveKeys: h.sensitiveKeys,
	}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		inner:         h.inner.WithGroup(name),
		attrs:         h.attrs,
		sensitiveKeys: h.sensitiveKeys,
	}
}

func (h *RedactingHandler) redactAttr(a slog.Attr) slog.Attr {
	if h.sensitiveKeys[a.Key] {
		// Resolve the value to get the actual underlying value, avoiding
		// slog's quoting behavior on Value.String().
		resolved := a.Value.Resolve()
		if resolved.Kind() == slog.KindString {
			if resolved.String() == "" {
				return a
			}
		}
		// Use a fixed redaction marker that does not leak secret length.
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

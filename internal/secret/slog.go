package secret

import (
	"context"
	"log/slog"
)

// LogHandler is the log egress filter (I-10). It wraps another
// [slog.Handler] and masks the message and every attribute value on the way
// through, so a record that carries a secret reaches the writer without it.
//
// The database keeps the original bytes and the tag; this is the other half of
// that bargain. The log is Phase 1's only egress - search, export and fixtures
// arrive later and pass through the same [Mask] and [MaskString].
//
// # Rebuild the record, never assign to the Attr
//
// [slog.Record.Attrs] hands its callback an [slog.Attr] by value. Assigning to
// a.Value inside the callback compiles, runs, changes nothing, and leaves the
// secret in the output - a redactor that returns its input and one that works
// look identical from outside. [LogHandler.Handle] therefore collects masked
// attributes and builds a new record with [slog.NewRecord] and
// [slog.Record.AddAttrs].
//
// # What it sees
//
// A value is masked against what [slog.Value.String] renders, after
// [slog.Value.Resolve], and is replaced only when masking changed something -
// so a number stays a number and a duration stays a duration in the output.
// The consequence is that a value whose encoding downstream differs from that
// rendering is not covered: a []byte the JSON handler base64-encodes, or a
// json.Marshaler with fields String does not show. Log a payload as a string.
//
// An attribute key is structure and is never masked, with the same single
// exception [Detect] makes: a key that names a credential tags its own value.
type LogHandler struct{ next slog.Handler }

// NewLogHandler returns a handler that masks every record on its way to next.
func NewLogHandler(next slog.Handler) *LogHandler { return &LogHandler{next: next} }

// Enabled is the handler below's decision.
func (h *LogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle masks r and passes on the rebuilt record.
func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, maskText("", r.Message), r.PC)
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, maskAttr(a))
		return true
	})
	out.AddAttrs(attrs...)
	return h.next.Handle(ctx, out)
}

// WithAttrs masks attrs now, because the handler below keeps them and this
// handler will not see them again on any later record.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	masked := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		masked[i] = maskAttr(a)
	}
	return &LogHandler{next: h.next.WithAttrs(masked)}
}

// WithGroup opens a group below. A group name is structure, like an object key,
// so nothing here masks it.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &LogHandler{next: h.next.WithGroup(name)}
}

// maskAttr returns a copy of a carrying the masked value. It returns rather
// than assigns: see the type's doc comment.
func maskAttr(a slog.Attr) slog.Attr {
	return slog.Attr{Key: a.Key, Value: maskValue(a.Key, a.Value)}
}

// maskValue masks v, which reached the record under key.
func maskValue(key string, v slog.Value) slog.Value {
	v = v.Resolve()
	if v.Kind() == slog.KindGroup {
		g := v.Group()
		out := make([]slog.Attr, len(g))
		for i, a := range g {
			out[i] = maskAttr(a)
		}
		return slog.GroupValue(out...)
	}
	rendered := v.String()
	if masked := maskText(key, rendered); masked != rendered {
		return slog.StringValue(masked)
	}
	return v
}

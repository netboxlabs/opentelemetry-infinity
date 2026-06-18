package runner

import (
	"log/slog"
	"testing"
)

func hasAttr(attrs []slog.Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

func TestMapCollectorLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"WARN":    slog.LevelWarn,
		"error":   slog.LevelError,
		"err":     slog.LevelError,
		"fatal":   slog.LevelError,
		"info":    slog.LevelInfo,
		"unknown": slog.LevelInfo,
		"":        slog.LevelInfo,
	}
	for in, want := range cases {
		if got := mapCollectorLevel(in); got != want {
			t.Errorf("mapCollectorLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestShouldSuppressCollectorLog(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"empty", "", false},
		{"whitespace", "   ", false},
		{"memfd warning", "2024-01-01\twarn\tinternal\tFailed to get executable path: lstat /memfd:foo", true},
		{"ordinary line", "2024-01-01\tinfo\tservice\tEverything is fine", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSuppressCollectorLog(tc.line); got != tc.want {
				t.Errorf("shouldSuppressCollectorLog(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestParseCollectorLog(t *testing.T) {
	t.Run("empty line", func(t *testing.T) {
		msg, level, attrs := parseCollectorLog("")
		if msg != "" || level != slog.LevelInfo || attrs != nil {
			t.Errorf("got (%q, %v, %v)", msg, level, attrs)
		}
	})

	t.Run("single field no tabs is trimmed", func(t *testing.T) {
		msg, level, attrs := parseCollectorLog("  just a message  ")
		if msg != "just a message" || level != slog.LevelInfo || attrs != nil {
			t.Errorf("got (%q, %v, %v)", msg, level, attrs)
		}
	})

	t.Run("level source and plain message", func(t *testing.T) {
		line := "2024-01-01T00:00:00Z\twarn\totelcol.receiver\tsomething happened"
		msg, level, attrs := parseCollectorLog(line)
		if msg != "something happened" {
			t.Errorf("msg = %q", msg)
		}
		if level != slog.LevelWarn {
			t.Errorf("level = %v", level)
		}
		if !hasAttr(attrs, "collector_source") {
			t.Errorf("expected collector_source attr, got %v", attrs)
		}
		if hasAttr(attrs, "collector_message") {
			t.Errorf("did not expect collector_message attr for plain text")
		}
	})

	t.Run("json message adds structured attr", func(t *testing.T) {
		line := "2024-01-01T00:00:00Z\tinfo\tsrc\t{\"k\":\"v\"}"
		msg, _, attrs := parseCollectorLog(line)
		if msg != `{"k":"v"}` {
			t.Errorf("msg = %q", msg)
		}
		if !hasAttr(attrs, "collector_message") {
			t.Errorf("expected collector_message attr, got %v", attrs)
		}
	})

	t.Run("json payload adds structured payload", func(t *testing.T) {
		line := "ts\tinfo\tsrc\tmessage\t{\"count\":3}"
		_, _, attrs := parseCollectorLog(line)
		if !hasAttr(attrs, "collector_payload") {
			t.Errorf("expected collector_payload attr, got %v", attrs)
		}
	})

	t.Run("non-json payload kept as string", func(t *testing.T) {
		line := "ts\tinfo\tsrc\tmessage\tplain payload text"
		_, _, attrs := parseCollectorLog(line)
		if !hasAttr(attrs, "collector_payload") {
			t.Errorf("expected collector_payload attr, got %v", attrs)
		}
	})
}

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewJSONLoggerProducesStructuredOutput(t *testing.T) {
	var buf bytes.Buffer

	logger, err := New("info", "json", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	logger.Info("hello world", "conversation_id", "abc-123")

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("expected valid JSON log line, got error: %v\noutput: %s", err, buf.String())
	}

	if got := entry[slog.MessageKey]; got != "hello world" {
		t.Fatalf("message = %v, want hello world", got)
	}

	if got := entry[slog.LevelKey]; got != "INFO" {
		t.Fatalf("level = %v, want INFO", got)
	}

	if got := entry["conversation_id"]; got != "abc-123" {
		t.Fatalf("conversation_id = %v, want abc-123", got)
	}
}

func TestNewTextLoggerProducesReadableOutput(t *testing.T) {
	var buf bytes.Buffer

	logger, err := New("info", "text", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	logger.Info("hello text", "turn_number", 5)

	output := buf.String()
	for _, want := range []string{"level=INFO", "msg=\"hello text\"", "turn_number=5"} {
		if !strings.Contains(output, want) {
			t.Fatalf("text log output %q does not contain %q", output, want)
		}
	}
}

func TestLevelFilteringSuppressesMessagesBelowThreshold(t *testing.T) {
	var buf bytes.Buffer

	logger, err := New("warn", "text", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Fatalf("debug message was emitted at warn level: %q", output)
	}
	if strings.Contains(output, "info message") {
		t.Fatalf("info message was emitted at warn level: %q", output)
	}
	if !strings.Contains(output, "warn message") {
		t.Fatalf("warn message missing from output: %q", output)
	}
}

func TestWithContextPropagatesParentFields(t *testing.T) {
	var buf bytes.Buffer

	base, err := New("info", "json", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	conversationLogger := WithContext(base, "conversation_id", "conv-1")
	turnLogger := WithContext(conversationLogger, "turn_number", 5)
	turnLogger.Info("turn started")

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("expected valid JSON log line, got error: %v\noutput: %s", err, buf.String())
	}

	if got := entry["conversation_id"]; got != "conv-1" {
		t.Fatalf("conversation_id = %v, want conv-1", got)
	}
	if got := entry["turn_number"]; got != float64(5) {
		t.Fatalf("turn_number = %v, want 5", got)
	}
}

func TestInitReconfiguresDefaultLogger(t *testing.T) {
	first, err := Init("info", "text")
	if err != nil {
		t.Fatalf("first Init returned error: %v", err)
	}
	if slog.Default() != first {
		t.Fatalf("first Init did not install the returned logger as default")
	}

	second, err := Init("debug", "json")
	if err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}
	if slog.Default() != second {
		t.Fatalf("second Init did not install the returned logger as default")
	}
}

func TestNewRejectsInvalidLevel(t *testing.T) {
	if _, err := New("loud", "text", &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid level error, got nil")
	}
}

func TestNewRejectsInvalidFormat(t *testing.T) {
	if _, err := New("info", "pretty", &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid format error, got nil")
	}
}

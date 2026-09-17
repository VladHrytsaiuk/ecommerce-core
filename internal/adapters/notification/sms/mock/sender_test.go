package mock

import (
	"context"
	"strings"
	"testing"
)

type recordingLogger struct{ lines []string }

func (l *recordingLogger) Infow(message string, fields ...interface{}) {
	parts := []string{message}
	for _, field := range fields {
		if value, ok := field.(string); ok {
			parts = append(parts, value)
		}
	}
	l.lines = append(l.lines, strings.Join(parts, " "))
}

func TestTheMockLogsTheTextWithTheNumberMasked(t *testing.T) {
	logger := &recordingLogger{}
	sender := New(logger)

	if err := sender.Send(context.Background(), "+380501234567", "Your code: 123456"); err != nil {
		t.Fatal(err)
	}
	if sent := sender.Sent(); len(sent) != 1 || sent[0].Phone != "+380501234567" {
		t.Fatalf("Sent() = %+v", sent)
	}
	line := logger.lines[0]
	if !strings.Contains(line, "123456") || !strings.Contains(line, "+380*******67") || strings.Contains(line, "501234567") {
		t.Fatalf("log line %q, want the code and a masked number", line)
	}
}

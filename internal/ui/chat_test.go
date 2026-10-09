package ui

import (
	"strings"
	"testing"

	"github.com/hocoder-agents/crush-bot/internal/crush"
)

func TestFormatTranscriptEmpty(t *testing.T) {
	s := formatTranscript(nil, "sophie")
	if !strings.Contains(s, "empty session") {
		t.Fatalf("%q", s)
	}
}

func TestFormatTranscriptRoles(t *testing.T) {
	s := formatTranscript([]crush.Line{
		{Role: "user", Text: "hi"},
		{Role: "assistant", Text: "hello"},
		{Role: "system", Text: "[message_bot]"},
	}, "sophie")
	if !strings.Contains(s, "hi") || !strings.Contains(s, "hello") {
		t.Fatalf("%q", s)
	}
	if !strings.Contains(s, "you") || !strings.Contains(s, "sophie") {
		t.Fatalf("missing role labels: %q", s)
	}
	s = formatTranscript([]crush.Line{{Role: "assistant", Text: "hello"}}, "")
	if !strings.Contains(s, "bot") {
		t.Fatalf("missing fallback label: %q", s)
	}
}

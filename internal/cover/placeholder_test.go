package cover

import (
	"strings"
	"testing"
)

func TestPlaceholder(t *testing.T) {
	data := Placeholder()
	if len(data) == 0 {
		t.Fatal("placeholder SVG is empty")
	}
	if !strings.HasPrefix(string(data), "<svg") {
		t.Errorf("placeholder = %q, want SVG", string(data[:min(20, len(data))]))
	}
	if PlaceholderMIME != "image/svg+xml" {
		t.Errorf("PlaceholderMIME = %q, want image/svg+xml", PlaceholderMIME)
	}
}

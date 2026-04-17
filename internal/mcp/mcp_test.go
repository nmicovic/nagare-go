package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInboxDirSanitizesSlashes(t *testing.T) {
	got := InboxDir("cosmo-ai/claude_01")
	if strings.Contains(filepath.Base(got), "/") {
		t.Errorf("InboxDir leaked slash into directory component: %q", got)
	}
	if filepath.Base(got) != "cosmo-ai__claude_01" {
		t.Errorf("got %q, want ...cosmo-ai__claude_01", got)
	}
}

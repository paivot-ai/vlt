package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	vlt "github.com/RamXX/vlt"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCmd    string
		wantParams map[string]string
		wantFlags  map[string]bool
	}{
		{
			name:       "read command",
			args:       []string{"vault=Claude", "read", "file=Session Operating Mode"},
			wantCmd:    "read",
			wantParams: map[string]string{"vault": "Claude", "file": "Session Operating Mode"},
			wantFlags:  map[string]bool{},
		},
		{
			name:       "create with silent flag",
			args:       []string{"vault=Claude", "create", "name=My Note", "path=_inbox/My Note.md", "content=# Hello", "silent"},
			wantCmd:    "create",
			wantParams: map[string]string{"vault": "Claude", "name": "My Note", "path": "_inbox/My Note.md", "content": "# Hello"},
			wantFlags:  map[string]bool{"silent": true},
		},
		{
			name:       "search command",
			args:       []string{"vault=Claude", "search", "query=architecture"},
			wantCmd:    "search",
			wantParams: map[string]string{"vault": "Claude", "query": "architecture"},
			wantFlags:  map[string]bool{},
		},
		{
			name:       "move command",
			args:       []string{"vault=Claude", "move", "path=_inbox/Note.md", "to=decisions/Note.md"},
			wantCmd:    "move",
			wantParams: map[string]string{"vault": "Claude", "path": "_inbox/Note.md", "to": "decisions/Note.md"},
			wantFlags:  map[string]bool{},
		},
		{
			name:       "property:set command",
			args:       []string{"vault=Claude", "property:set", "file=Note", "name=status", "value=archived"},
			wantCmd:    "property:set",
			wantParams: map[string]string{"vault": "Claude", "file": "Note", "name": "status", "value": "archived"},
			wantFlags:  map[string]bool{},
		},
		{
			name:       "content with equals sign",
			args:       []string{"vault=Claude", "create", "name=Note", "path=_inbox/Note.md", "content=key=value"},
			wantCmd:    "create",
			wantParams: map[string]string{"vault": "Claude", "name": "Note", "path": "_inbox/Note.md", "content": "key=value"},
			wantFlags:  map[string]bool{},
		},
		{
			name:       "quoted value stripping",
			args:       []string{`vault="Claude"`, "read", `file="My Note"`},
			wantCmd:    "read",
			wantParams: map[string]string{"vault": "Claude", "file": "My Note"},
			wantFlags:  map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, params, flags := parseArgs(tt.args)

			if cmd != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tt.wantCmd)
			}

			for k, want := range tt.wantParams {
				got, ok := params[k]
				if !ok {
					t.Errorf("missing param %q", k)
				} else if got != want {
					t.Errorf("param[%q] = %q, want %q", k, got, want)
				}
			}
			if len(params) != len(tt.wantParams) {
				t.Errorf("got %d params, want %d", len(params), len(tt.wantParams))
			}

			for k := range tt.wantFlags {
				if !flags[k] {
					t.Errorf("missing flag %q", k)
				}
			}
			if len(flags) != len(tt.wantFlags) {
				t.Errorf("got %d flags, want %d", len(flags), len(tt.wantFlags))
			}
		})
	}
}

func TestDispatchWriteRejectsEmptyContent(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "Note.md")
	os.WriteFile(notePath, []byte("---\ntype: note\n---\n\n# Body\n"), 0644)

	v, err := vlt.Open(dir)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	// No content= param, no stdin -- dispatch should reject
	params := map[string]string{"file": "Note"}
	err = dispatchWrite(v, params, false)
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
	if !strings.Contains(err.Error(), "no content provided") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Verify original body is untouched
	data, _ := os.ReadFile(notePath)
	if !strings.Contains(string(data), "# Body") {
		t.Error("note body was modified despite empty content rejection")
	}
}

func TestDispatchWriteAcceptsContent(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "Note.md")
	os.WriteFile(notePath, []byte("---\ntype: note\n---\n\n# Old Body\n"), 0644)

	v, err := vlt.Open(dir)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	params := map[string]string{"file": "Note", "content": "# New Body\n"}
	err = dispatchWrite(v, params, false)
	if err != nil {
		t.Fatalf("write with content: %v", err)
	}

	data, _ := os.ReadFile(notePath)
	got := string(data)
	if !strings.Contains(got, "# New Body") {
		t.Error("new body not written")
	}
	if strings.Contains(got, "# Old Body") {
		t.Error("old body still present")
	}
}

func TestDispatchCreateRejectsEmptyContent(t *testing.T) {
	dir := t.TempDir()
	v, err := vlt.Open(dir)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	params := map[string]string{"name": "Empty", "path": "Empty.md"}
	err = dispatchCreate(v, params, false, false)
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
	if !strings.Contains(err.Error(), "no content provided") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Verify no file was created
	if _, statErr := os.Stat(filepath.Join(dir, "Empty.md")); statErr == nil {
		t.Error("empty file was created despite rejection")
	}
}

func TestDispatchCreateAcceptsFrontmatterOnly(t *testing.T) {
	dir := t.TempDir()
	v, err := vlt.Open(dir)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	fm := "---\ntype: note\nstatus: active\n---\n"
	params := map[string]string{"name": "FMOnly", "path": "FMOnly.md", "content": fm}
	err = dispatchCreate(v, params, false, false)
	if err != nil {
		t.Fatalf("create with frontmatter-only: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "FMOnly.md"))
	if !strings.Contains(string(data), "type: note") {
		t.Error("frontmatter not written")
	}
}

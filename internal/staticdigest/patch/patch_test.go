package patch

import (
	"reflect"
	"testing"
)

func TestParseHunks(t *testing.T) {
	patch := "@@ -198,23 +198,37 @@ func promptSystemd() error {\n" +
		" \t\treturn nil\n" +
		"-\tself, err := os.Executable()\n" +
		"+\tself, err := os.Executable()\n" +
		"+\tif err != nil {\n" +
		"@@ -371,6 +371,12 @@ func (m *Manager) waitReady() error {\n" +
		"+\t// Check local cache first\n"

	got := Parse(patch)

	if len(got.Removed) != 1 || got.Removed[0] != "\tself, err := os.Executable()" {
		t.Errorf("Removed = %q", got.Removed)
	}
	if len(got.Added) != 3 {
		t.Fatalf("Added = %q, want 3 lines", got.Added)
	}
	wantCtx := []string{"func promptSystemd() error {", "func (m *Manager) waitReady() error {"}
	if !reflect.DeepEqual(got.FuncContexts, wantCtx) {
		t.Errorf("FuncContexts = %q, want %q", got.FuncContexts, wantCtx)
	}
}

func TestParseTruncated(t *testing.T) {
	// GitHub truncates long patches mid-hunk and git-digest's prompt builder
	// appends an ellipsis; both must be tolerated without dropping earlier hunks.
	patch := "@@ -1,5 +1,5 @@ func A() {}\n" +
		"-old one\n" +
		"+new one\n" +
		"…\n"
	got := Parse(patch)
	if len(got.Added) != 1 || got.Added[0] != "new one" ||
		len(got.Removed) != 1 || got.Removed[0] != "old one" {
		t.Errorf("truncated parse = %+v", got)
	}
}

func TestParseNoNewlineMarker(t *testing.T) {
	got := Parse("@@ -1,2 +1,2 @@\n-a\n\\ No newline at end of file\n+b\n")
	if len(got.Added) != 1 || len(got.Removed) != 1 {
		t.Errorf("no-newline marker mishandled: %+v", got)
	}
}

func TestParseEmpty(t *testing.T) {
	got := Parse("")
	if len(got.Added)+len(got.Removed)+len(got.FuncContexts) != 0 {
		t.Errorf("empty patch should parse to zero value, got %+v", got)
	}
}

func TestOrderedLines(t *testing.T) {
	got := Parse("@@ -1,4 +1,4 @@\n ctx\n-del\n+add\n")
	want := []struct {
		kind Kind
		text string
	}{{Context, "ctx"}, {Removed, "del"}, {Added, "add"}}
	if len(got.Lines) != len(want) {
		t.Fatalf("Lines = %+v", got.Lines)
	}
	for i, w := range want {
		if got.Lines[i].Kind != w.kind || got.Lines[i].Text != w.text {
			t.Errorf("line %d = %+v, want %c %q", i, got.Lines[i], w.kind, w.text)
		}
	}
}

func TestFuncContextDedup(t *testing.T) {
	patch := "@@ -1,2 +1,2 @@ func serve() error {\n+a\n@@ -9,2 +9,3 @@ func serve() error {\n+b\n"
	got := Parse(patch)
	if len(got.FuncContexts) != 1 || got.FuncContexts[0] != "func serve() error {" {
		t.Errorf("dedup failed: %q", got.FuncContexts)
	}
}

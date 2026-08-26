package deps

import (
	"testing"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/patch"
)

func extract(t *testing.T, path, patchText string) []facts.DependencyChange {
	t.Helper()
	return Extract(path, patch.Parse(patchText))
}

func TestGoModBumpAndAdd(t *testing.T) {
	got := extract(t, "go.mod",
		"@@ -5,4 +5,5 @@\n"+
			"-\tgithub.com/google/go-github v62.0.0+incompatible\n"+
			"+\tgithub.com/google/go-github v63.0.0+incompatible\n"+
			"+\tgithub.com/newkid/thing v1.2.3\n"+
			"\tgithub.com/stays/put v1.0.0\n")
	if len(got) != 2 {
		t.Fatalf("got %d changes: %+v", len(got), got)
	}
	for _, d := range got {
		switch d.Name {
		case "github.com/google/go-github":
			if d.Action != "bumped" || d.Old != "v62.0.0+incompatible" || d.New != "v63.0.0+incompatible" {
				t.Errorf("bump wrong: %+v", d)
			}
		case "github.com/newkid/thing":
			if d.Action != "added" {
				t.Errorf("add wrong: %+v", d)
			}
		default:
			t.Errorf("unexpected dep %q", d.Name)
		}
	}
}

func TestPackageJsonDevBump(t *testing.T) {
	got := extract(t, "web/package.json",
		"@@ -12,3 +12,3 @@\n"+
			"   \"dependencies\": {\n"+
			"-    \"vite\": \"^5.0.0\",\n"+
			"+    \"vite\": \"^6.0.0\",\n")
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	d := got[0]
	if d.Name != "vite" || d.Action != "bumped" || d.Ecosystem != "node" || !d.Major {
		t.Errorf("got %+v", d)
	}
}

func TestRequirementsAdded(t *testing.T) {
	got := extract(t, "requirements.txt",
		"@@ -1,2 +1,3 @@\n requests==2.31.0\n+uvicorn[standard]==0.30.0\n")
	if len(got) != 1 || got[0].Name != "uvicorn" || got[0].New != "0.30.0" ||
		got[0].Action != "added" || got[0].Ecosystem != "python" {
		t.Errorf("got %+v", got)
	}
}

func TestCargoMajorBump(t *testing.T) {
	got := extract(t, "Cargo.toml",
		"@@ -8,2 +8,2 @@\n-serde = \"1.0.200\"\n+serde = \"2.0.0\"\n")
	if len(got) != 1 || got[0].Action != "bumped" || got[0].Ecosystem != "rust" || !got[0].Major {
		t.Errorf("got %+v", got)
	}
}

func TestRemovedDep(t *testing.T) {
	got := extract(t, "go.mod", "@@ -7,3 +7,2 @@\n-\tgithub.com/old/pkg v1.0.0\n")
	if len(got) != 1 || got[0].Action != "removed" || got[0].Old != "v1.0.0" {
		t.Errorf("got %+v", got)
	}
}

func TestLockfilesIgnored(t *testing.T) {
	for _, p := range []string{"go.sum", "package-lock.json", "yarn.lock", "Cargo.lock", "poetry.lock"} {
		if got := extract(t, p, "@@ -1,1 +1,2 @@\n+anything == 1.0\n"); got != nil {
			t.Errorf("%s should be ignored, got %+v", p, got)
		}
	}
}

func TestNonManifestIgnored(t *testing.T) {
	if got := extract(t, "cmd/main.go", "@@ -1,1 +1,2 @@\n+x := 1\n"); got != nil {
		t.Errorf("non-manifest should be ignored, got %+v", got)
	}
}

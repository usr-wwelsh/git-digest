package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
)

func fixCommit() facts.CommitFacts {
	return facts.CommitFacts{
		SHA:     "ac4a9c1",
		Subject: "systemd absolute path",
		Type:    "fix",
		Files: []facts.FileChange{{
			Path:         "cmd/setup.go",
			Additions:    38,
			Deletions:    6,
			FuncContexts: []string{"func promptSystemd() error {"},
		}},
	}
}

func TestSingleFixRepo(t *testing.T) {
	out := Digest([]facts.RepoFacts{{
		Name:    "usr-wwelsh/git-digest",
		Commits: []facts.CommitFacts{fixCommit()},
	}})
	want := strings.Join([]string{
		"## Summary",
		"",
		"1 commit across 1 repo. Biggest change in usr-wwelsh/git-digest: fixed systemd absolute path (promptSystemd()).",
		"",
		"## Per-Repo Activity",
		"",
		"### usr-wwelsh/git-digest",
		"",
		"Fixed systemd absolute path (promptSystemd()).",
		"",
	}, "\n")
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestLeadingVerbNotDoubled(t *testing.T) {
	c := fixCommit()
	c.Type = "feat"
	c.Subject = "Add rate limiting to login"
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{c}}})
	if strings.Contains(out, "Added add rate") {
		t.Errorf("verb doubled: %s", out)
	}
	if !strings.Contains(out, "Added rate limiting to login") {
		t.Errorf("subject mangled: %s", out)
	}
}

func TestDepAndRiskSentences(t *testing.T) {
	c := facts.CommitFacts{
		SHA:      "d49b6c9",
		Subject:  "upgrade web tooling",
		Type:     "chore",
		Breaking: true,
		Deps: []facts.DependencyChange{
			{Name: "vite", Old: "^5.0.0", New: "^6.0.0", Action: "bumped", Ecosystem: "node", Major: true},
		},
		RiskFlags: []string{"pipeline config"},
		Files:     []facts.FileChange{{Path: "package.json"}},
	}
	out := Digest([]facts.RepoFacts{{Name: "r/web", Commits: []facts.CommitFacts{c}}})
	for _, want := range []string{
		"Tidied web tooling",
		"Dependency updates: bumped vite ^5.0.0 -> ^6.0.0 (major).",
		"Watch: pipeline config; includes breaking changes.",
		"breaking changes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestManyCommitsCappedWithCount(t *testing.T) {
	var commits []facts.CommitFacts
	for i := 0; i < 6; i++ {
		commits = append(commits, facts.CommitFacts{
			SHA:     "aaaaaaaa",
			Subject: fmt.Sprintf("change number %d", i),
			Type:    "fix",
			Files:   []facts.FileChange{{Path: "a.go", Additions: i + 1}},
		})
	}
	out := Digest([]facts.RepoFacts{{Name: "r/multi", Commits: commits}})
	if !strings.Contains(out, "Shipped 6 commits:") {
		t.Errorf("missing count lead: %s", out)
	}
	if got := strings.Count(out, "Fixed "); got > 4 {
		t.Errorf("too many clauses (%d): %s", got, out)
	}
}

func TestEmptyInput(t *testing.T) {
	if out := Digest(nil); out != "" {
		t.Errorf("empty input should give empty digest, got %q", out)
	}
}

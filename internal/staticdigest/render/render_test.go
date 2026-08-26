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
	}}, "")
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
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{c}}}, "")
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
	out := Digest([]facts.RepoFacts{{Name: "r/web", Commits: []facts.CommitFacts{c}}}, "")
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
	out := Digest([]facts.RepoFacts{{Name: "r/multi", Commits: commits}}, "")
	if !strings.Contains(out, "Shipped 6 commits:") {
		t.Errorf("missing count lead: %s", out)
	}
	if got := strings.Count(out, "Fixed "); got > 4 {
		t.Errorf("too many clauses (%d): %s", got, out)
	}
}

func genesisCommitFixture() facts.CommitFacts {
	return facts.CommitFacts{
		SHA:     "0000001",
		Subject: "init",
		Type:    "other",
		Genesis: true,
		Files:   []facts.FileChange{{Path: "README.md"}, {Path: "go.mod"}, {Path: "main.go"}},
	}
}

func TestGenesisCommitSoleRepo(t *testing.T) {
	out := Digest([]facts.RepoFacts{{
		Name:    "usr-wwelsh/new-thing",
		Commits: []facts.CommitFacts{genesisCommitFixture()},
	}}, "")
	if !strings.Contains(out, "New repo: bootstrapped with an init commit (3 files).") {
		t.Errorf("missing genesis phrasing:\n%s", out)
	}
	if !strings.Contains(out, "usr-wwelsh/new-thing launched as a new repo.") {
		t.Errorf("missing summary new-repo mention:\n%s", out)
	}
	if strings.Contains(out, "Worked on init") {
		t.Errorf("genesis commit should not be listed as a normal clause:\n%s", out)
	}
}

func TestGenesisCommitAmongOtherCommits(t *testing.T) {
	out := Digest([]facts.RepoFacts{{
		Name:    "r/x",
		Commits: []facts.CommitFacts{genesisCommitFixture(), fixCommit()},
	}}, "")
	if !strings.Contains(out, "New repo: bootstrapped with an init commit (3 files).") {
		t.Errorf("missing genesis prefix:\n%s", out)
	}
	if !strings.Contains(out, "Fixed systemd absolute path") {
		t.Errorf("other commit dropped:\n%s", out)
	}
}

func TestBiggestChangeSkipsGenesis(t *testing.T) {
	g := genesisCommitFixture()
	g.Files = []facts.FileChange{{Path: "vendor/dump.go", Additions: 5000}}
	out := Digest([]facts.RepoFacts{{
		Name:    "r/x",
		Commits: []facts.CommitFacts{g, fixCommit()},
	}}, "")
	if strings.Contains(out, "Biggest change in r/x: worked on init") {
		t.Errorf("genesis commit should not be picked as biggest change:\n%s", out)
	}
	if !strings.Contains(out, "Biggest change in r/x: fixed systemd absolute path") {
		t.Errorf("non-genesis commit should be picked as biggest change:\n%s", out)
	}
}

func TestMultipleNewRepos(t *testing.T) {
	out := Digest([]facts.RepoFacts{
		{Name: "r/a", Commits: []facts.CommitFacts{genesisCommitFixture()}},
		{Name: "r/b", Commits: []facts.CommitFacts{genesisCommitFixture()}},
	}, "")
	if !strings.Contains(out, "New repos launched: r/a, r/b.") {
		t.Errorf("missing multi-repo genesis summary:\n%s", out)
	}
}

func TestNotableDocCommitGuaranteedMention(t *testing.T) {
	var commits []facts.CommitFacts
	for i := 0; i < 7; i++ {
		commits = append(commits, facts.CommitFacts{
			SHA:     fmt.Sprintf("c%d", i),
			Subject: fmt.Sprintf("change number %d", i),
			Type:    "fix",
			Files:   []facts.FileChange{{Path: "a.go", Additions: 100 + i}},
		})
	}
	commits = append(commits, facts.CommitFacts{
		SHA:     "postmortem1",
		Subject: "add postmortem, retire finetuning route from release path",
		Type:    "docs",
		Notable: true,
		Files:   []facts.FileChange{{Path: "README.md", Additions: 30}},
	})
	out := Digest([]facts.RepoFacts{{Name: "r/digest-finetune", Commits: commits}}, "")
	if !strings.Contains(out, "documented postmortem, retire finetuning route from release path") {
		t.Errorf("postmortem commit should be guaranteed a mention despite low churn score:\n%s", out)
	}
}

func TestNotePrependedVerbatimToSummary(t *testing.T) {
	out := Digest([]facts.RepoFacts{{
		Name:    "usr-wwelsh/git-digest",
		Commits: []facts.CommitFacts{fixCommit()},
	}}, "Juggling a birthday party but managed to get a little work in today")
	want := "## Summary\n\nJuggling a birthday party but managed to get a little work in today. " +
		"1 commit across 1 repo. Biggest change in usr-wwelsh/git-digest: fixed systemd absolute path (promptSystemd())."
	if !strings.HasPrefix(out, want) {
		t.Errorf("note should lead the summary verbatim:\ngot:\n%s\nwant prefix:\n%s", out, want)
	}
}

func TestNoteAlreadyPunctuatedNotDoubled(t *testing.T) {
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{fixCommit()}}},
		"Shipped from the airport.")
	if strings.Contains(out, "airport..") {
		t.Errorf("note punctuation doubled:\n%s", out)
	}
}

func TestNoteWithoutAnyCommits(t *testing.T) {
	out := Digest(nil, "Family thing today, no code.")
	if !strings.HasPrefix(out, "## Summary\n\nFamily thing today, no code.") {
		t.Errorf("note-only digest malformed:\n%s", out)
	}
	if strings.Contains(out, "## Per-Repo Activity") {
		t.Errorf("no repo section expected when there's no activity:\n%s", out)
	}
}

func TestEmptyInput(t *testing.T) {
	if out := Digest(nil, ""); out != "" {
		t.Errorf("empty input should give empty digest, got %q", out)
	}
}

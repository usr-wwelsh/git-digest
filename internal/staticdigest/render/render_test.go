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
		"1 commit across 1 repo. Biggest change in git-digest: fixed systemd absolute path.",
		"",
		"## Per-Repo Activity",
		"",
		"### git-digest",
		"",
		"Fixed systemd absolute path.",
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
	if !strings.Contains(out, "new-thing launched as a new repo.") {
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
	if strings.Contains(out, "Biggest change in x: worked on init") {
		t.Errorf("genesis commit should not be picked as biggest change:\n%s", out)
	}
	if !strings.Contains(out, "Biggest change in x: fixed systemd absolute path") {
		t.Errorf("non-genesis commit should be picked as biggest change:\n%s", out)
	}
}

func TestMultipleNewRepos(t *testing.T) {
	out := Digest([]facts.RepoFacts{
		{Name: "r/a", Commits: []facts.CommitFacts{genesisCommitFixture()}},
		{Name: "r/b", Commits: []facts.CommitFacts{genesisCommitFixture()}},
	}, "")
	if !strings.Contains(out, "New repos launched: a, b.") {
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
	if !strings.Contains(out, "documented postmortem, retired finetuning route from release path") {
		t.Errorf("postmortem commit should be guaranteed a mention despite low churn score:\n%s", out)
	}
}

func TestCommaSegmentVerbsConjugated(t *testing.T) {
	c := fixCommit()
	c.Type = "feat"
	c.Subject = "recognize init commits and postmortems, dampen data-dump churn"
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{c}}}, "")
	if strings.Contains(out, "added recognize") {
		t.Errorf("leading verb should be stripped regardless of type:\n%s", out)
	}
	want := "Added init commits and postmortems, dampened data-dump churn"
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

func TestVerbStrippedAcrossTypes(t *testing.T) {
	for _, tc := range []struct{ typ, subj, want string }{
		{"fix", "wire retry into download worker", "Fixed retry into download worker"},
		{"feat", "dampen reward hacking on short rollouts", "Added reward hacking on short rollouts"},
	} {
		c := fixCommit()
		c.Type = tc.typ
		c.Subject = tc.subj
		out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{c}}}, "")
		if !strings.Contains(out, tc.want) {
			t.Errorf("[%s] missing %q:\n%s", tc.typ, tc.want, out)
		}
	}
}

func TestNotePrependedVerbatimToSummary(t *testing.T) {
	out := Digest([]facts.RepoFacts{{
		Name:    "usr-wwelsh/git-digest",
		Commits: []facts.CommitFacts{fixCommit()},
	}}, "Juggling a birthday party but managed to get a little work in today")
	want := "## Summary\n\nJuggling a birthday party but managed to get a little work in today. " +
		"1 commit across 1 repo. Biggest change in git-digest: fixed systemd absolute path."
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

func TestRepoBaseNameStripsOwner(t *testing.T) {
	if got := repoBaseName("usr-wwelsh/git-digest"); got != "git-digest" {
		t.Errorf("got %q, want %q", got, "git-digest")
	}
}

func TestRepoBaseNameKeepsBareName(t *testing.T) {
	if got := repoBaseName("git-digest"); got != "git-digest" {
		t.Errorf("got %q, want %q", got, "git-digest")
	}
}

func TestNoParenthesizedCitationsWithoutMinedTokens(t *testing.T) {
	c := fixCommit()
	c.Files[0].FuncContexts = []string{"import (", "func promptSystemd() error {"}
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{c}}}, "")
	if strings.Contains(out, "(") {
		t.Errorf("parenthesized identifiers read as robotic; none should appear without mined tokens:\n%s", out)
	}
}

func TestEvidenceOnlyFromMinedTokens(t *testing.T) {
	c := fixCommit()
	c.Subject = "deterministic generation without an LLM"
	c.Files[0].AddedLines = []string{
		"\tstatic := flag.Bool(\"static\", false, \"generate deterministically\")",
	}
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{c}}}, "")
	if !strings.Contains(out, "(-static)") {
		t.Errorf("mined flag missing from evidence:\n%s", out)
	}
}

func TestThematicMergeSameTypeAndArea(t *testing.T) {
	commits := []facts.CommitFacts{
		{SHA: "1", Subject: "hunk context extraction", Type: "fix",
			Files: []facts.FileChange{{Path: "internal/staticdigest/patch/patch.go"}}},
		{SHA: "2", Subject: "keyword blocklist", Type: "fix",
			Files: []facts.FileChange{{Path: "internal/staticdigest/symbols/symbols.go"}}},
		{SHA: "3", Subject: "unrelated worker retry", Type: "fix",
			Files: []facts.FileChange{{Path: "cmd/worker/run.go"}, {Path: "cmd/worker/backoff.go"}}},
		{SHA: "4", Subject: "another unrelated thing", Type: "fix",
			Files: []facts.FileChange{{Path: "cmd/serve/serve.go"}}},
		{SHA: "5", Subject: "yet another unrelated thing", Type: "fix",
			Files: []facts.FileChange{{Path: "web/app/main.ts"}}},
	}
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: commits}}, "")
	if !strings.Contains(out, "fixed hunk context extraction and keyword blocklist") {
		t.Errorf("related commits should merge into one clause:\n%s", out)
	}
}

func TestNoMergeWithoutSharedArea(t *testing.T) {
	commits := []facts.CommitFacts{
		{SHA: "1", Subject: "first thing", Type: "fix",
			Files: []facts.FileChange{{Path: "cmd/a/a.go"}}},
		{SHA: "2", Subject: "second thing", Type: "fix",
			Files: []facts.FileChange{{Path: "web/b/c.ts"}}},
		{SHA: "3", Subject: "third thing", Type: "fix",
			Files: []facts.FileChange{{Path: "lib/d/e.py"}}},
		{SHA: "4", Subject: "fourth thing", Type: "fix",
			Files: []facts.FileChange{{Path: "misc/f/f.go"}}},
	}
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: commits}}, "")
	if strings.Contains(out, " and ") {
		t.Errorf("unrelated commits merged:\n%s", out)
	}
}

func TestScopeTopicLead(t *testing.T) {
	var commits []facts.CommitFacts
	for i := 0; i < 5; i++ {
		commits = append(commits, facts.CommitFacts{
			SHA:     fmt.Sprintf("s%d", i),
			Subject: fmt.Sprintf("change number %d", i),
			Type:    "feat",
			Scope:   "staticdigest",
			Files:   []facts.FileChange{{Path: fmt.Sprintf("pkg%d/file%d.go", i, i)}},
		})
	}
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: commits}}, "")
	if !strings.Contains(out, "Staticdigest: ") {
		t.Errorf("missing scope topic lead:\n%s", out)
	}
	if strings.Contains(out, "Shipped 5 commits") {
		t.Errorf("count lead should be replaced by scope lead:\n%s", out)
	}
}

func TestRiskFlagConsequenceTail(t *testing.T) {
	c := fixCommit()
	c.RiskFlags = []string{"security-sensitive paths"}
	c.Files[0].Path = "internal/auth/login.go"
	out := Digest([]facts.RepoFacts{{Name: "r/x", Commits: []facts.CommitFacts{c}}}, "")
	if !strings.Contains(out, "hardening security-sensitive paths") {
		t.Errorf("missing consequence tail:\n%s", out)
	}
}

func TestMultiRepoSummaryLeadsWithDominantRepo(t *testing.T) {
	quiet := fixCommit()
	busy := []facts.CommitFacts{
		{SHA: "b1", Subject: "the big rewrite", Type: "refactor", Score: 90,
			Files: []facts.FileChange{{Path: "core/engine.go", Additions: 400}}},
		{SHA: "b2", Subject: "small followup", Type: "chore", Score: 10,
			Files: []facts.FileChange{{Path: "core/engine.go", Additions: 4}}},
	}
	out := Digest([]facts.RepoFacts{
		{Name: "r/quiet", Commits: []facts.CommitFacts{quiet}},
		{Name: "r/busy", Commits: busy},
	}, "")
	if !strings.Contains(out, "busy led the day:") {
		t.Errorf("summary should lead with the dominant repo:\n%s", out)
	}
	if !strings.Contains(out, "Refactored the big rewrite") {
		t.Errorf("lead should cite the dominant repo's top commit:\n%s", out)
	}
	if strings.Contains(out, "Biggest change in") {
		t.Errorf("redundant biggest-change sentence should be gone on multi-repo days:\n%s", out)
	}
	if !strings.Contains(out, "commits across 2 repos") {
		t.Errorf("counts should survive:\n%s", out)
	}
}

func TestDayCharacterizationMultiRepoOnly(t *testing.T) {
	fixHeavy := make([]facts.CommitFacts, 3)
	for i := range fixHeavy {
		fixHeavy[i] = facts.CommitFacts{SHA: fmt.Sprintf("f%d", i), Subject: fmt.Sprintf("bug %d", i), Type: "fix",
			Files: []facts.FileChange{{Path: fmt.Sprintf("a/%d.go", i)}}}
	}
	multi := Digest([]facts.RepoFacts{
		{Name: "r/one", Commits: fixHeavy},
		{Name: "r/two", Commits: []facts.CommitFacts{fixCommit()}},
	}, "")
	variants := []string{"Stabilization was the theme of the day.", "A hardening-and-fixes kind of day."}
	if !strings.Contains(multi, variants[0]) && !strings.Contains(multi, variants[1]) {
		t.Errorf("multi-repo day should get a characterization closer:\n%s", multi)
	}
	if again := Digest([]facts.RepoFacts{
		{Name: "r/one", Commits: fixHeavy},
		{Name: "r/two", Commits: []facts.CommitFacts{fixCommit()}},
	}, ""); again != multi {
		t.Errorf("characterization must be deterministic:\n%s\n%s", multi, again)
	}

	single := Digest([]facts.RepoFacts{
		{Name: "r/one", Commits: fixHeavy},
	}, "")
	for _, v := range variants {
		if strings.Contains(single, v) {
			t.Errorf("single-repo day should not get a characterization closer:\n%s", single)
		}
	}
}

func TestSingleRepoSummaryUnchanged(t *testing.T) {
	out := Digest([]facts.RepoFacts{{Name: "r/solo", Commits: []facts.CommitFacts{fixCommit()}}}, "")
	if !strings.Contains(out, "1 commit across 1 repo. Biggest change in solo:") {
		t.Errorf("single-repo summary shape changed unexpectedly:\n%s", out)
	}
}

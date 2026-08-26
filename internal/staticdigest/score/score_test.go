package score

import (
	"testing"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
)

func base() facts.CommitFacts {
	return facts.CommitFacts{
		Type:  "chore",
		Files: []facts.FileChange{{Path: "README.md", Additions: 2, Deletions: 1}},
	}
}

func TestTrivialChoreLowScore(t *testing.T) {
	if s := Of(base()); s > 20 {
		t.Errorf("trivial chore scored %.0f, want <= 20", s)
	}
}

func TestBreakingSecurityFixScoresHigh(t *testing.T) {
	c := facts.CommitFacts{
		Type:      "fix",
		Breaking:  true,
		RiskFlags: []string{"security-sensitive paths"},
		Deps: []facts.DependencyChange{
			{Name: "openssl", Action: "bumped", Major: true},
		},
		Files: []facts.FileChange{
			{Path: "internal/auth/jwt.go", Additions: 80, Deletions: 40},
			{Path: "internal/auth/session.go", Additions: 30, Deletions: 20},
		},
	}
	if s := Of(c); s < 60 {
		t.Errorf("breaking security fix scored %.0f, want >= 60", s)
	}
}

func TestChurnRaisesScore(t *testing.T) {
	small := base()
	big := base()
	big.Files = []facts.FileChange{{Path: "a.go", Additions: 200, Deletions: 100}}
	if Of(big) <= Of(small) {
		t.Errorf("churn should raise score: big=%.0f small=%.0f", Of(big), Of(small))
	}
}

func TestDataDumpChurnDiscounted(t *testing.T) {
	code := facts.CommitFacts{
		Type:  "feat",
		Files: []facts.FileChange{{Path: "internal/foo.go", Additions: 400}},
	}
	dump := facts.CommitFacts{
		Type:  "feat",
		Files: []facts.FileChange{{Path: "testdata/fixtures.json", Additions: 400}},
	}
	if Of(dump) >= Of(code) {
		t.Errorf("json dump should score lower than equivalent code churn: dump=%.0f code=%.0f", Of(dump), Of(code))
	}
}

func TestLockfileChurnIgnored(t *testing.T) {
	c := facts.CommitFacts{
		Type:  "chore",
		Files: []facts.FileChange{{Path: "package-lock.json", Additions: 2000, Deletions: 1900}},
	}
	if s := Of(c); s > 20 {
		t.Errorf("lockfile-only commit scored %.0f, want <= 20 (churn should be ignored)", s)
	}
}

func TestManifestChurnStillCounts(t *testing.T) {
	small := facts.CommitFacts{Type: "chore", Files: []facts.FileChange{{Path: "package.json", Additions: 2, Deletions: 1}}}
	big := facts.CommitFacts{Type: "chore", Files: []facts.FileChange{{Path: "package.json", Additions: 200, Deletions: 100}}}
	if Of(big) <= Of(small) {
		t.Errorf("manifest churn should still raise score: big=%.0f small=%.0f", Of(big), Of(small))
	}
}

func TestClampAtHundred(t *testing.T) {
	c := facts.CommitFacts{
		Type:      "fix",
		Breaking:  true,
		RiskFlags: []string{"a", "b", "c"},
		Deps:      []facts.DependencyChange{{Major: true}, {Major: true}},
		Files: []facts.FileChange{
			{Path: "x", Additions: 500, Deletions: 500},
			{Path: "y"}, {Path: "z"}, {Path: "w"}, {Path: "v"},
		},
	}
	if s := Of(c); s > 100 {
		t.Errorf("score %.0f exceeds 100", s)
	}
}

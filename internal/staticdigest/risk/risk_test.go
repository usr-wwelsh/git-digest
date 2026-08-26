package risk

import (
	"testing"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
)

func fc(path string, added ...string) facts.FileChange {
	return facts.FileChange{Path: path, AddedLines: added}
}

func TestAuthPathFlagged(t *testing.T) {
	got := Flags([]facts.FileChange{fc("internal/auth/jwt.go")})
	want := "security-sensitive paths"
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %q, want [%q]", got, want)
	}
}

func TestMigrationFlagged(t *testing.T) {
	got := Flags([]facts.FileChange{fc("db/migrations/0002_users.sql")})
	if len(got) != 1 || got[0] != "schema migration" {
		t.Errorf("got %q", got)
	}
}

func TestPipelineFlagged(t *testing.T) {
	for _, p := range []string{".github/workflows/ci.yml", ".gitlab-ci.yml", "Jenkinsfile"} {
		if got := Flags([]facts.FileChange{fc(p)}); len(got) != 1 {
			t.Errorf("%s: got %q", p, got)
		}
	}
}

func TestPublicAPISurface(t *testing.T) {
	if got := Flags([]facts.FileChange{fc("api/handlers/users.go"), fc("web/src/lib/api.js")}); len(got) != 1 {
		t.Errorf("got %q", got)
	}
}

func TestAWSSecretInAddedLine(t *testing.T) {
	got := Flags([]facts.FileChange{fc("cmd/deploy.go", `key := "AKIAIOSFODNN7EXAMPLE"`)})
	found := false
	for _, f := range got {
		if f == "possible secret in added lines" {
			found = true
		}
	}
	if !found {
		t.Errorf("got %q", got)
	}
}

func TestPrivateKeyHeader(t *testing.T) {
	got := Flags([]facts.FileChange{fc("tls/key.pem", "-----BEGIN RSA PRIVATE KEY-----")})
	found := false
	for _, f := range got {
		if f == "possible secret in added lines" {
			found = true
		}
	}
	if !found {
		t.Errorf("got %q", got)
	}
}

func TestDropTable(t *testing.T) {
	got := Flags([]facts.FileChange{fc("db/schema.sql", "DROP TABLE users;")})
	if len(got) != 1 || got[0] != "destructive schema operation" {
		t.Errorf("got %q", got)
	}
}

func TestBenignCommitClean(t *testing.T) {
	got := Flags([]facts.FileChange{
		fc("README.md", "# hello\n"),
		fc("src/util/format.go", "func F(x int) int { return x }\n"),
	})
	if len(got) != 0 {
		t.Errorf("benign files flagged: %q", got)
	}
}

func TestDuplicateFlagsCollapsed(t *testing.T) {
	got := Flags([]facts.FileChange{fc("auth/a.go"), fc("auth/b.go")})
	if len(got) != 1 {
		t.Errorf("got %q", got)
	}
}

// Package risk applies deterministic path and content rules to flag commits
// that warrant attention: security-sensitive paths, migrations, pipelines,
// public API surface, and possible secrets in added lines.
package risk

import (
	"path"
	"regexp"
	"strings"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
)

const (
	flagSecurity   = "security-sensitive paths"
	flagMigration  = "schema migration"
	flagPipeline   = "pipeline config"
	flagAPI        = "public API surface"
	flagSecret     = "possible secret in added lines"
	flagDestructiv = "destructive schema operation"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]+`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|password|passwd)\s*[:=]\s*["'][^"'$\{]{8,}["']`),
}

var destructivePattern = regexp.MustCompile(`(?i)\b(DROP\s+(TABLE|COLUMN|DATABASE)|TRUNCATE\s+TABLE)\b`)

func riskyPath(p string) (string, bool) {
	lp := strings.ToLower(p)
	base := path.Base(lp)
	dir := path.Dir(lp)

	switch {
	case containsAny(lp, "auth", "login", "session", "jwt", "token", "crypto",
		"password", "passwd", "oauth", "secret"):
		return flagSecurity, true
	case strings.Contains(base, "migrat") || containsSeg(dir, "migrat"):
		return flagMigration, true
	case strings.HasPrefix(dir, ".github/workflows"), base == ".gitlab-ci.yml",
		base == "jenkinsfile", dir == ".circleci":
		return flagPipeline, true
	case dir == "api" || strings.HasPrefix(lp, "api/") || strings.Contains(dir, "/api"):
		return flagAPI, true
	default:
		return "", false
	}
}

func Flags(files []facts.FileChange) []string {
	set := map[string]bool{}
	var out []string
	add := func(flag string) {
		if flag != "" && !set[flag] {
			set[flag] = true
			out = append(out, flag)
		}
	}
	for _, f := range files {
		if flag, ok := riskyPath(f.Path); ok {
			add(flag)
		}
		for _, line := range f.AddedLines {
			for _, re := range secretPatterns {
				if re.MatchString(line) {
					add(flagSecret)
				}
			}
			if destructivePattern.MatchString(line) {
				add(flagDestructiv)
			}
		}
	}
	return out
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func containsSeg(dir, sub string) bool {
	for _, seg := range strings.Split(dir, "/") {
		if strings.Contains(seg, sub) {
			return true
		}
	}
	return false
}

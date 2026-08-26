// Package score assigns a transparent 0–100 importance heuristic to commit
// facts. No learning, no opacity: type weight + breaking bonus + churn +
// dependency and risk bonuses, clamped.
package score

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
)

var baseType = map[string]float64{
	"fix":      30,
	"feat":     28,
	"perf":     26,
	"refactor": 18,
	"other":    12,
	"docs":     8,
	"test":     8,
	"style":    8,
	"chore":    8,
	"build":    8,
	"ci":       8,
	"revert":   8,
}

// dataExt marks file extensions that are typically bulk data (fixtures,
// exports, dumps) rather than logic — their line counts inflate churn
// without a matching rise in functional significance.
var dataExt = map[string]bool{
	".json": true, ".jsonl": true, ".ndjson": true,
	".csv": true, ".tsv": true, ".parquet": true,
}

// manifestBase excludes dependency/config manifests from the data-file
// discount: their churn is small in practice but does signal real change.
var manifestBase = map[string]bool{
	"package.json": true, "composer.json": true, "tsconfig.json": true,
	"jsconfig.json": true, "manifest.json": true, "biome.json": true,
	".eslintrc.json": true,
}

// generatedBase marks lockfiles: mechanically regenerated, huge diffs,
// ~zero functional signal.
var generatedBase = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"go.sum": true, "cargo.lock": true, "poetry.lock": true, "uv.lock": true,
	"composer.lock": true, "gemfile.lock": true,
}

// lowSignalChurn reports whether a file's line churn should be excluded from
// the score: a big diff here (data dump, lockfile) doesn't mean a big
// functional change.
func lowSignalChurn(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if generatedBase[base] {
		return true
	}
	if manifestBase[base] {
		return false
	}
	return dataExt[strings.ToLower(filepath.Ext(base))]
}

func Of(c facts.CommitFacts) float64 {
	s := baseType[c.Type]
	if c.Breaking {
		s += 25
	}
	if c.Notable {
		s += 15
	}
	var add, del int
	for _, f := range c.Files {
		if lowSignalChurn(f.Path) {
			continue
		}
		add += f.Additions
		del += f.Deletions
	}
	s += math.Min(float64(add+del)/10, 20)
	for _, d := range c.Deps {
		s += 6
		if d.Major {
			s += 10
		}
	}
	s += float64(len(c.RiskFlags)) * 8
	if len(c.Files) >= 5 {
		s += 5
	}
	return math.Min(s, 100)
}

// Apply fills in the Score field for every commit.
func Apply(commits []facts.CommitFacts) {
	for i := range commits {
		commits[i].Score = Of(commits[i])
	}
}

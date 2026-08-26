// Package score assigns a transparent 0–100 importance heuristic to commit
// facts. No learning, no opacity: type weight + breaking bonus + churn +
// dependency and risk bonuses, clamped.
package score

import (
	"math"

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

func Of(c facts.CommitFacts) float64 {
	s := baseType[c.Type]
	if c.Breaking {
		s += 25
	}
	var add, del int
	for _, f := range c.Files {
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

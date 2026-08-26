// Package pipeline wires the stages together: git-digest activity JSON in,
// scored repo facts out. Classification, hunk parsing, dependency extraction,
// import mining, risk flags, and scoring all happen here — render then turns
// the facts into markdown.
package pipeline

import (
	"encoding/json"
	"io"
	"path"
	"strings"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/classify"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/deps"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/patch"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/risk"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/score"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/symbols"
)

type activityFile struct {
	Filename  string `json:"filename"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

type activityCommit struct {
	SHA     string         `json:"sha"`
	Message string         `json:"message"`
	URL     string         `json:"url"`
	Files   []activityFile `json:"files"`
}

type activityRepo struct {
	Commits []activityCommit `json:"commits"`
}

type activity map[string]activityRepo

func Build(r io.Reader) ([]facts.RepoFacts, error) {
	var act activity
	if err := json.NewDecoder(r).Decode(&act); err != nil {
		return nil, err
	}
	var out []facts.RepoFacts
	for name, ar := range act {
		repo := facts.RepoFacts{Name: name}
		for _, ac := range ar.Commits {
			repo.Commits = append(repo.Commits, buildCommit(ac))
		}
		out = append(out, repo)
	}
	return out, nil
}

func buildCommit(ac activityCommit) facts.CommitFacts {
	res := classify.Parse(ac.Message)
	c := facts.CommitFacts{
		SHA:      ac.SHA,
		URL:      ac.URL,
		Subject:  res.Subject,
		Type:     res.Type,
		Scope:    res.Scope,
		Breaking: res.Breaking,
	}

	areaSet := map[string]bool{}
	importSet := map[string]bool{}

	for _, af := range ac.Files {
		p := patch.Parse(af.Patch)
		fc := facts.FileChange{
			Path:         af.Filename,
			Additions:    af.Additions,
			Deletions:    af.Deletions,
			AddedLines:   p.Added,
			RemovedLines: p.Removed,
			FuncContexts: p.FuncContexts,
		}
		c.Files = append(c.Files, fc)
		c.Deps = append(c.Deps, deps.Extract(af.Filename, p)...)
		for _, imp := range symbols.Imports(af.Filename, p.Added) {
			if !importSet[imp] {
				importSet[imp] = true
				c.Imports = append(c.Imports, imp)
			}
		}
		if a := areaOf(af.Filename); a != "" && !areaSet[a] {
			areaSet[a] = true
			c.Areas = append(c.Areas, a)
		}
	}

	c.RiskFlags = risk.Flags(c.Files)
	c.Score = score.Of(c)
	return c
}

// areaOf reduces a file path to its first one or two directory segments.
func areaOf(p string) string {
	dir := path.Dir(p)
	segs := strings.Split(dir, "/")
	if len(segs) == 0 || segs[0] == "." {
		return ""
	}
	if len(segs) >= 2 {
		return segs[0] + "/" + segs[1]
	}
	return segs[0]
}

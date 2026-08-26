// Package facts defines the data model flowing through the pipeline:
// activity JSON in, CommitFacts/RepoFacts out, rendered to markdown.
package facts

import (
	"path"
	"strings"
)

type FileChange struct {
	Path         string   `json:"path"`
	Additions    int      `json:"additions"`
	Deletions    int      `json:"deletions"`
	AddedLines   []string `json:"added_lines,omitempty"`
	RemovedLines []string `json:"removed_lines,omitempty"`
	FuncContexts []string `json:"func_contexts,omitempty"`
}

type DependencyChange struct {
	Name      string `json:"name"`
	Old       string `json:"old,omitempty"`
	New       string `json:"new,omitempty"`
	Action    string `json:"action"` // added | removed | bumped | downgraded
	Ecosystem string `json:"ecosystem"`
	Major     bool   `json:"major,omitempty"` // major-version jump
}

type CommitFacts struct {
	SHA       string             `json:"sha"`
	URL       string             `json:"url,omitempty"`
	Subject   string             `json:"subject"`
	Type      string             `json:"type"`
	Scope     string             `json:"scope,omitempty"`
	Breaking  bool               `json:"breaking,omitempty"`
	Files     []FileChange       `json:"files"`
	Deps      []DependencyChange `json:"deps,omitempty"`
	Areas     []string           `json:"areas,omitempty"`
	RiskFlags []string           `json:"risk_flags,omitempty"`
	Imports   []string           `json:"imports,omitempty"` // new modules imported by changed code
	Genesis   bool               `json:"genesis,omitempty"` // repo's own "init" commit
	Notable   bool               `json:"notable,omitempty"` // narratively significant despite a small diff (postmortem, incident report)
	Score     float64            `json:"score"`
}

type RepoFacts struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Commits     []CommitFacts `json:"commits"`
}

// AreaOf reduces a file path to its first one or two directory segments —
// the coarse bucket a change belongs to.
func AreaOf(p string) string {
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

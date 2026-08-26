// Package classify parses commit messages into type, scope, breaking flag,
// and cleaned subject — conventional commits first, verb heuristics as fallback.
package classify

import (
	"regexp"
	"strings"
)

var knownTypes = map[string]bool{
	"feat": true, "fix": true, "refactor": true, "perf": true,
	"docs": true, "test": true, "chore": true, "build": true,
	"ci": true, "style": true, "revert": true,
}

type Result struct {
	Type     string
	Scope    string
	Breaking bool
	Subject  string
}

var conventional = regexp.MustCompile(`^(\w+)(?:\(([^)]*)\))?(!)?:\s*(.*)$`)

func Parse(message string) Result {
	firstLine := message
	if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
		firstLine = firstLine[:i]
	}
	firstLine = strings.TrimSpace(firstLine)

	if m := conventional.FindStringSubmatch(firstLine); m != nil {
		word := strings.ToLower(m[1])
		typ := "other"
		subject := strings.TrimSpace(m[4])
		if knownTypes[word] {
			typ = word
		}
		return Result{Type: typ, Scope: m[2], Breaking: m[3] == "!", Subject: subject}
	}

	return Result{Type: guessType(firstLine), Subject: firstLine}
}

func guessType(subject string) string {
	lower := strings.ToLower(subject)
	switch {
	case strings.HasPrefix(lower, "merge "), strings.HasPrefix(lower, "revert "):
		return "other"
	case strings.HasPrefix(lower, "fix"), strings.HasPrefix(lower, "repair"),
		strings.HasPrefix(lower, "handle "), strings.Contains(lower, "bug"):
		return "fix"
	case strings.HasPrefix(lower, "add"), strings.HasPrefix(lower, "introduce"),
		strings.HasPrefix(lower, "support "), strings.HasPrefix(lower, "implement"):
		return "feat"
	case strings.HasPrefix(lower, "bump"), strings.HasPrefix(lower, "update deps"),
		strings.HasPrefix(lower, "upgrade"):
		return "chore"
	default:
		return "other"
	}
}

// Package patch parses the per-file unified diff hunks GitHub's API attaches
// to commit file stats. It tolerates truncated patches (GitHub cuts long ones
// mid-hunk) and extracts added/removed lines plus git's enclosing-function
// hunk context, which is where "what functions changed" comes from.
package patch

import (
	"regexp"
	"strings"
)

type Kind byte

const (
	Context Kind = ' '
	Added   Kind = '+'
	Removed Kind = '-'
)

type Line struct {
	Kind Kind
	Text string
}

type Parsed struct {
	Added        []string
	Removed      []string
	FuncContexts []string
	Lines        []Line
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@(?: (.*))?$`)

func Parse(patchText string) Parsed {
	var p Parsed
	seenCtx := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(patchText, "\n"), "\n") {
		if strings.HasPrefix(line, "@@ ") {
			if m := hunkHeader.FindStringSubmatch(line); m != nil && m[1] != "" {
				ctx := strings.TrimSpace(m[1])
				if ctx != "" && !seenCtx[ctx] {
					seenCtx[ctx] = true
					p.FuncContexts = append(p.FuncContexts, ctx)
				}
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			p.Lines = append(p.Lines, Line{Added, line[1:]})
			p.Added = append(p.Added, line[1:])
		case strings.HasPrefix(line, "-"):
			p.Lines = append(p.Lines, Line{Removed, line[1:]})
			p.Removed = append(p.Removed, line[1:])
		case strings.HasPrefix(line, "\\"):
			// no-newline marker
		default:
			text := strings.TrimPrefix(line, " ")
			p.Lines = append(p.Lines, Line{Context, text})
		}
	}
	return p
}

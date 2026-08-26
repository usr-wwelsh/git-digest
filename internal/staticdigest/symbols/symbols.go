// Package symbols mines cheap structural signals that don't need an AST:
// enclosing-function names from git hunk headers, and new imports from added
// lines, keyed by file extension.
package symbols

import (
	"path"
	"regexp"
	"strings"
)

var identBeforeParen = regexp.MustCompile(`([A-Za-z_$][A-Za-z0-9_$.]*)\s*\(`)

var keywords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true,
	"return": true, "catch": true, "else": true, "do": true,
}

// CleanFuncName reduces a git hunk-header function context to "name()".
// Returns "" when the context isn't recognizably a function signature.
func CleanFuncName(ctx string) string {
	ctx = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(ctx), "{"))
	// Take the last identifier-( pair: method receivers like
	// "func (m *Manager) waitReady()" put an earlier "(" on the receiver.
	locs := identBeforeParen.FindAllStringSubmatchIndex(ctx, -1)
	if len(locs) == 0 {
		return ""
	}
	name := ctx[locs[len(locs)-1][2]:locs[len(locs)-1][3]]
	if keywords[strings.ToLower(name)] {
		return ""
	}
	return name + "()"
}

var (
	pyFrom    = regexp.MustCompile(`^from\s+([A-Za-z0-9_.]+)\s+import\b`)
	pyImp     = regexp.MustCompile(`^import\s+([A-Za-z0-9_.]+)`)
	goImp     = regexp.MustCompile(`"([^"]+)"`)
	jsFrom    = regexp.MustCompile(`(?:^|\bfrom)\s*['"]([^'"]+)['"]`)
	jsReq     = regexp.MustCompile(`require\(\s*['"]([^'"]+)['"]\)`)
	rsUse     = regexp.MustCompile(`^use\s+([A-Za-z0-9_:]+)`)
	sourceExt = map[string]bool{
		".py": true, ".go": true, ".rs": true,
		".js": true, ".jsx": true, ".ts": true, ".tsx": true,
		".mjs": true, ".cjs": true, ".svelte": true, ".vue": true,
	}
)

// Imports extracts module names from added lines of a source file, deduped
// in order of appearance. Non-source files yield nil.
func Imports(filePath string, added []string) []string {
	ext := strings.ToLower(path.Ext(filePath))
	if !sourceExt[ext] {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, line := range added {
		s := strings.TrimSpace(line)
		switch ext {
		case ".py":
			if m := pyFrom.FindStringSubmatch(s); m != nil {
				add(m[1])
			} else if m := pyImp.FindStringSubmatch(s); m != nil {
				add(m[1])
			}
		case ".go":
			if m := goImp.FindStringSubmatch(s); m != nil {
				add(m[1])
			}
		case ".rs":
			if m := rsUse.FindStringSubmatch(s); m != nil {
				add(m[1])
			}
		default: // js/ts family
			if m := jsFrom.FindStringSubmatch(s); m != nil {
				add(m[1])
			} else if m := jsReq.FindStringSubmatch(s); m != nil {
				add(m[1])
			}
		}
	}
	return out
}

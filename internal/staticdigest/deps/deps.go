// Package deps extracts dependency deltas from manifest-file patches.
// The +/- lines of a unified diff on go.mod, package.json, requirements.txt,
// Cargo.toml or Gemfile are the dependency change — no TOML/JSON schema
// parsing needed. Lockfiles are skipped (they're noise at digest scale).
package deps

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/patch"
)

type ecosystem struct {
	manifests map[string]bool
	name      string
}

var (
	goEco     = ecosystem{map[string]bool{"go.mod": true}, "go"}
	nodeEco   = ecosystem{map[string]bool{"package.json": true}, "node"}
	pythonEco = ecosystem{map[string]bool{"requirements.txt": true, "pyproject.toml": true}, "python"}
	rustEco   = ecosystem{map[string]bool{"cargo.toml": true}, "rust"}
	rubyEco   = ecosystem{map[string]bool{"gemfile": true}, "ruby"}
)

func ecoFor(path string) *ecosystem {
	base := filepath.Base(strings.ToLower(path))
	for _, e := range []*ecosystem{&goEco, &nodeEco, &pythonEco, &rustEco, &rubyEco} {
		if e.manifests[base] {
			return e
		}
	}
	return nil
}

var lockfiles = map[string]bool{
	"go.sum": true, "package-lock.json": true, "yarn.lock": true,
	"pnpm-lock.yaml": true, "Cargo.lock": true, "poetry.lock": true,
	"uv.lock": true, "composer.lock": true, "Gemfile.lock": true,
}

// Extract returns dependency changes found in a parsed patch of path.
func Extract(path string, p patch.Parsed) []facts.DependencyChange {
	base := filepath.Base(strings.ToLower(path))
	if lockfiles[base] {
		return nil
	}
	eco := ecoFor(path)
	if eco == nil {
		return nil
	}

	var removed, added []depLine
	switch {
	case goEco.manifests[base]:
		removed, added = scanGo(p)
	case nodeEco.manifests[base]:
		removed, added = scanNode(p)
	case pythonEco.manifests[base]:
		removed, added = scanPython(base, p)
	case rustEco.manifests[base]:
		removed, added = scanRust(p)
	case rubyEco.manifests[base]:
		removed, added = scanRuby(p)
	}

	old := map[string]string{}
	for _, d := range removed {
		if _, dup := old[d.name]; !dup {
			old[d.name] = d.version
		}
	}
	newSet := map[string]string{}
	for _, d := range added {
		if _, dup := newSet[d.name]; !dup {
			newSet[d.name] = d.version
		}
	}

	var out []facts.DependencyChange
	seen := map[string]bool{}
	emit := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		o, n := old[name], newSet[name]
		d := facts.DependencyChange{Name: name, Old: o, New: n, Ecosystem: eco.name}
		switch {
		case o != "" && n != "":
			c := compareVersions(o, n)
			if c < 0 {
				d.Action = "bumped"
				d.Major = majorJump(o, n)
			} else if c > 0 {
				d.Action = "downgraded"
			} else {
				return // version unchanged (e.g. moved sections)
			}
		case n != "":
			d.Action = "added"
		default:
			d.Action = "removed"
			d.New = ""
		}
		out = append(out, d)
	}
	for _, d := range added {
		emit(d.name)
	}
	for _, d := range removed {
		emit(d.name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type depLine struct {
	name    string
	version string
}

func cleanVer(v string) string {
	return strings.Trim(strings.TrimSpace(v), "\"',")
}

var goRequire = regexp.MustCompile(`^\s*(?:require\s+)?([\w./~-]+)\s+(v[\w.\-+]+)`)

func scanGo(p patch.Parsed) (removed, added []depLine) {
	handle := func(lines []string, into *[]depLine) {
		for _, l := range lines {
			if m := goRequire.FindStringSubmatch(l); m != nil {
				*into = append(*into, depLine{m[1], m[2]})
			}
		}
	}
	handle(p.Removed, &removed)
	handle(p.Added, &added)
	return removed, added
}

var nodeKV = regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]*)"`)

func scanNode(p patch.Parsed) (removed, added []depLine) {
	// Walk the ordered line stream so context lines can tell us which JSON
	// section ("dependencies" vs "devDependencies") a change belongs to.
	inDeps := false
	for _, l := range p.Lines {
		s := strings.TrimSpace(l.Text)
		switch {
		case strings.Contains(s, "dependencies") && strings.HasSuffix(s, "{"):
			inDeps = true
		case inDeps && (strings.HasPrefix(s, "}") || strings.HasPrefix(s, "]")):
			inDeps = false
		case !inDeps:
		case l.Kind == patch.Added:
			if m := nodeKV.FindStringSubmatch(s); m != nil {
				added = append(added, depLine{m[1], cleanVer(m[2])})
			}
		case l.Kind == patch.Removed:
			if m := nodeKV.FindStringSubmatch(s); m != nil {
				removed = append(removed, depLine{m[1], cleanVer(m[2])})
			}
		}
	}
	return removed, added
}

var pyReq = regexp.MustCompile(`^([A-Za-z0-9_.\-]+)(?:\[[^\]]*\])?\s*(==|>=|~=|!=|===)\s*([0-9][\w.*]*)`)
var pyKV = regexp.MustCompile(`^([A-Za-z0-9_.\-]+)\s*=\s*"([^"]+)"`)
var pyArrayItem = regexp.MustCompile(`"?([A-Za-z0-9_.\-]+)(?:\[[^\]]*\])?(==|>=|~=)\s*([0-9][\w.*]*)"?`)

func scanPython(base string, p patch.Parsed) (removed, added []depLine) {
	scan := func(lines []string, into *[]depLine) {
		for _, l := range lines {
			s := strings.TrimSpace(l)
			if base == "requirements.txt" {
				if m := pyReq.FindStringSubmatch(s); m != nil {
					*into = append(*into, depLine{m[1], m[3]})
				}
				continue
			}
			// pyproject: poetry-style key = "range", or PEP 621 array item
			if m := pyKV.FindStringSubmatch(s); m != nil {
				*into = append(*into, depLine{m[1], cleanVer(m[2])})
				continue
			}
			if m := pyArrayItem.FindStringSubmatch(strings.Trim(s, `",`)); m != nil {
				*into = append(*into, depLine{m[1], m[3]})
			}
		}
	}
	scan(p.Removed, &removed)
	scan(p.Added, &added)
	return removed, added
}

var rustKV = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*=\s*"([^"]+)"`)
var rustInline = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*=\s*\{\s*version\s*=\s*"([^"]+)"`)

func scanRust(p patch.Parsed) (removed, added []depLine) {
	scan := func(lines []string, into *[]depLine) {
		for _, l := range lines {
			s := strings.TrimSpace(l)
			if strings.HasPrefix(s, "[") { // section header like [dependencies]
				continue
			}
			if m := rustInline.FindStringSubmatch(s); m != nil {
				*into = append(*into, depLine{m[1], m[2]})
				continue
			}
			if m := rustKV.FindStringSubmatch(s); m != nil {
				*into = append(*into, depLine{m[1], m[2]})
			}
		}
	}
	scan(p.Removed, &removed)
	scan(p.Added, &added)
	return removed, added
}

var rubyGem = regexp.MustCompile(`^\s*gem\s+['"]([^'"]+)['"](?:\s*,\s*['"]([^'"]+)['"])?`)

func scanRuby(p patch.Parsed) (removed, added []depLine) {
	scan := func(lines []string, into *[]depLine) {
		for _, l := range lines {
			if m := rubyGem.FindStringSubmatch(l); m != nil {
				*into = append(*into, depLine{m[1], cleanVer(m[2])})
			}
		}
	}
	scan(p.Removed, &removed)
	scan(p.Added, &added)
	return removed, added
}

// compareVersions compares dotted numeric versions after stripping range
// markers and v prefixes. Negative means a < b. Non-numeric segments compare
// lexically; missing segments count as zero.
func compareVersions(a, b string) int {
	as, bs := splitVer(a), splitVer(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		an, aerr := strconv.Atoi(av)
		bn, berr := strconv.Atoi(bv)
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		default:
			if av != bv {
				if av < bv {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}

func splitVer(v string) []string {
	v = strings.TrimLeft(strings.TrimSpace(v), "v^~>=< ")
	v = strings.SplitN(v, "+", 2)[0]
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '_' })
	return parts
}

func majorJump(a, b string) bool {
	as, bs := splitVer(a), splitVer(b)
	if as == nil || bs == nil {
		return false
	}
	an, aerr1 := strconv.Atoi(as[0])
	bn, aerr2 := strconv.Atoi(bs[0])
	return aerr1 == nil && aerr2 == nil && an != bn
}

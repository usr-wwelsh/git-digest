// Package render stitches extracted facts into digest markdown with plain
// templates. Every phrase traces back to an input fact — the renderer cannot
// invent filenames, functions, versions, or behaviors.
package render

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
)

const maxMentioned = 3

var verbPast = map[string]string{
	"feat":     "Added",
	"fix":      "Fixed",
	"refactor": "Refactored",
	"perf":     "Optimized",
	"docs":     "Documented",
	"test":     "Tested",
	"chore":    "Tidied",
	"build":    "Updated",
	"ci":       "Updated",
	"style":    "Polished",
	"revert":   "Reverted",
	"other":    "Worked on",
}

// imperativePast maps common commit-message imperative verbs to past tense.
// A leading verb is stripped outright (the template verb carries it); verbs
// heading later comma-separated segments are conjugated so multi-action
// subjects stay grammatical under one template verb.
var imperativePast = map[string]string{
	"add": "added", "allow": "allowed", "avoid": "avoided",
	"backport": "backported", "bump": "bumped", "cache": "cached",
	"cap": "capped", "catch": "caught", "clean": "cleaned",
	"close": "closed", "delete": "deleted", "document": "documented",
	"drop": "dropped", "dampen": "dampened", "dedupe": "deduped",
	"enable": "enabled", "escape": "escaped", "expose": "exposed",
	"extract": "extracted", "extend": "extended", "fix": "fixed",
	"gate": "gated", "guard": "guarded", "handle": "handled",
	"harden": "hardened", "hoist": "hoisted", "ignore": "ignored",
	"improve": "improved", "implement": "implemented", "inline": "inlined",
	"introduce": "introduced", "limit": "limited", "log": "logged",
	"make": "made", "merge": "merged", "migrate": "migrated",
	"memoize": "memoized", "move": "moved", "normalize": "normalized",
	"optimize": "optimized", "pin": "pinned", "polish": "polished",
	"prevent": "prevented", "recognize": "recognized", "remove": "removed",
	"rename": "renamed", "replace": "replaced", "repair": "repaired",
	"resolve": "resolved", "restrict": "restricted", "restructure": "restructured",
	"retire": "retired", "retry": "retried", "revert": "reverted",
	"rework": "reworked", "sanitize": "sanitized", "set": "set",
	"ship": "shipped", "show": "showed", "simplify": "simplified",
	"skip": "skipped", "split": "split", "stream": "streamed",
	"strip": "stripped", "support": "supported", "switch": "switched",
	"test": "tested", "tidy": "tidied", "truncate": "truncated",
	"tune": "tuned", "unify": "unified", "update": "updated",
	"upgrade": "upgraded", "use": "used", "validate": "validated",
	"wire": "wired", "adjust": "adjusted", "align": "aligned", "apply": "applied",
	"bundle": "bundled", "collapse": "collapsed", "combine": "combined",
	"configure": "configured", "consolidate": "consolidated", "convert": "converted",
	"correct": "corrected", "create": "created", "define": "defined",
	"derive": "derived", "detect": "detected", "disable": "disabled",
	"display": "displayed", "emit": "emitted", "ensure": "ensured",
	"exclude": "excluded", "expand": "expanded", "fetch": "fetched",
	"filter": "filtered", "flatten": "flattened", "fold": "folded",
	"forward": "forwarded", "generate": "generated", "hide": "hid",
	"highlight": "highlighted", "include": "included", "install": "installed",
	"keep": "kept", "load": "loaded", "lock": "locked", "mute": "muted",
	"parse": "parsed", "populate": "populated", "prefer": "preferred",
	"preserve": "preserved", "propagate": "propagated", "provide": "provided",
	"prune": "pruned", "put": "put", "raise": "raised", "read": "read",
	"rebuild": "rebuilt", "reduce": "reduced", "refine": "refined",
	"register": "registered", "reject": "rejected", "render": "rendered",
	"reorder": "reordered", "reorganize": "reorganized", "require": "required",
	"reset": "reset", "resize": "resized", "respect": "respected",
	"restore": "restored", "reuse": "reused", "reveal": "revealed",
	"reword": "reworded", "rewrite": "rewrote", "run": "ran", "save": "saved",
	"scrub": "scrubbed", "select": "selected", "send": "sent", "separate": "separated",
	"serve": "served", "shorten": "shortened", "sort": "sorted", "surface": "surfaced",
	"sync": "synced", "throttle": "throttled", "tighten": "tightened",
	"toggle": "toggled", "track": "tracked", "treat": "treated", "trigger": "triggered",
	"trim": "trimmed", "tweak": "tweaked", "unblock": "unblocked", "warn": "warned",
	"widen": "widened", "wrap": "wrapped", "write": "wrote", "clarify": "clarified",
	"regenerate": "regenerated", "rebase": "rebased", "stop": "stopped",
}

var pastForm = func() map[string]bool {
	m := make(map[string]bool, len(imperativePast))
	for _, p := range imperativePast {
		m[p] = true
	}
	return m
}()

var riskPhrases = map[string]string{
	"destructive schema operation": "destructive schema operations",
	"security-sensitive paths":     "security-sensitive paths",
	"schema migration":             "a schema migration",
	"public API surface":           "the public API surface",
	"pipeline config":              "CI pipeline config",
}

var acronyms = map[string]string{
	"url": "URL", "urls": "URLs", "uri": "URI", "api": "API", "apis": "APIs",
	"ci": "CI", "cli": "CLI", "ui": "UI", "ux": "UX", "json": "JSON",
	"yaml": "YAML", "html": "HTML", "css": "CSS", "sql": "SQL", "http": "HTTP",
	"https": "HTTPS", "tls": "TLS", "ssh": "SSH", "dns": "DNS", "cpu": "CPU",
	"gpu": "GPU", "jwt": "JWT", "pwa": "PWA", "id": "ID", "ids": "IDs",
}

var releaseRe = regexp.MustCompile(`(?i)^(?:release|version|bump version to|bump to|tag)\s+v?(\d+(?:\.\d+)*[\w.+-]*)$`)

var notVerbFollowers = map[string]bool{
	"for": true, "of": true, "is": true, "are": true, "was": true, "in": true,
}

var dayPhrases = map[string][]string{
	"fix":      {"Stabilization was the theme of the day.", "A hardening-and-fixes kind of day."},
	"feat":     {"Feature shipping defined the day.", "A build-something-new kind of day."},
	"docs":     {"Wrap-up and documentation closed the day.", "A documentation-heavy day."},
	"perf":     {"Performance work carried the day."},
	"chore":    {"Housekeeping filled the day.", "Cleanup and upkeep were the story."},
	"refactor": {"Restructuring filled the day.", "A cleanup-and-restructure kind of day."},
}

// soloDayTemplates carry a single-commit day's whole story in one sentence:
// %[1]s is the repo name, %[2]s the commit's clause (verb + body, evidence
// and risk tail included). Replacing the flat "1 commit across 1 repo."
// count plus a "Biggest change in X: ..." sentence that just repeats the
// per-repo paragraph below — with one integrated, varied line keeps the same
// grounded detail without the deadpan double-statement.
var soloDayTemplates = []string{
	"Quiet day — %s %s.",
	"Just one commit today: %s %s.",
	"The lone commit landed in %s, which %s.",
	"One commit did the work: %s %s.",
	"Small day, but %s %s.",
}

// Digest renders repo facts into the markdown shape git-digest publishes:
// "## Summary" plus one "### repo" section per repository. note, when set,
// is a manual note (e.g. offline work not captured in GitHub) reproduced
// verbatim as the Summary's opening sentence — grounded facts never rewrite
// or paraphrase it.
func Digest(repos []facts.RepoFacts, note string) string {
	var total int
	for _, r := range repos {
		total += len(r.Commits)
	}
	note = strings.TrimSpace(note)
	if total == 0 && note == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Summary\n\n")
	if note != "" {
		sb.WriteString(note)
		if last := note[len(note)-1]; last != '.' && last != '!' && last != '?' {
			sb.WriteString(".")
		}
		if total > 0 {
			sb.WriteString(" ")
		}
	}
	if total == 0 {
		sb.WriteString("\n\n")
		return sb.String()
	}

	multi := len(repos) > 1
	var best *scored
	if !multi {
		best = biggestChange(repos)
	}
	solo := best != nil && total == 1
	if solo {
		sb.WriteString(soloCommitSentence(best))
	} else {
		if lead := leadSentences(repos, multi); lead != "" {
			sb.WriteString(lead + " ")
		}
		sb.WriteString(summaryLine(total, len(repos)))
	}
	if nr := genesisRepoNames(repos); len(nr) == 1 {
		sb.WriteString(fmt.Sprintf(" %s launched as a new repo.", nr[0]))
	} else if len(nr) > 1 {
		sb.WriteString(fmt.Sprintf(" New repos launched: %s.", strings.Join(nr, ", ")))
	}
	if b := breakingRepos(repos); len(b) > 0 {
		sb.WriteString(" Breaking changes flagged in ")
		sb.WriteString(strings.Join(b, ", "))
		sb.WriteString(".")
	}
	if closer := dayCharacterization(repos, variationSeed(repos, total)); closer != "" && multi {
		sb.WriteString(" " + closer)
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Per-Repo Activity\n")
	for _, r := range repos {
		sb.WriteString("\n### " + repoBaseName(r.Name) + "\n\n")
		sb.WriteString(repoParagraph(r))
		sb.WriteString("\n")
	}
	return sb.String()
}

// repoBaseName strips a "owner/repo" full name down to just "repo" for
// display in prose and headers — the owner segment is redundant once every
// digest is scoped to a single GitHub account, and full names read as noise.
func repoBaseName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func summaryLine(total, nrepos int) string {
	return fmt.Sprintf("%s across %s.", plural(total, "commit"), plural(nrepos, "repo"))
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

type scored struct {
	repo   facts.RepoFacts
	commit facts.CommitFacts
	sum    float64
}

func biggestChange(repos []facts.RepoFacts) *scored {
	var best *scored
	for _, r := range repos {
		for _, c := range withoutGenesis(r.Commits) {
			if best == nil || c.Score > best.commit.Score {
				bc := c
				br := r
				best = &scored{repo: br, commit: bc}
			}
		}
	}
	return best
}

// dominantRepo picks the repo with the highest total non-genesis score and
// remembers its top commit for the summary lead sentence.
func dominantRepo(repos []facts.RepoFacts) *scored {
	var best *scored
	for _, r := range repos {
		commits := withoutGenesis(r.Commits)
		if len(commits) == 0 {
			continue
		}
		var sum float64
		top := commits[0]
		for _, c := range commits {
			sum += c.Score
			if c.Score > top.Score {
				top = c
			}
		}
		if best == nil || sum > best.sum {
			best = &scored{repo: r, commit: top, sum: sum}
		}
	}
	return best
}

// repoSummaryClause renders a repo's top units plus any releases as one
// lowercase verb phrase for the Summary.
func repoSummaryClause(r facts.RepoFacts, maxUnits int) string {
	st := storyOf(r.Commits)
	var parts []string
	for i, u := range st.units {
		if i >= maxUnits {
			break
		}
		parts = append(parts, lowerFirst(unitClause(u)))
	}
	if rc := st.releaseClause(); rc != "" {
		parts = append(parts, lowerFirst(rc))
	}
	return joinList(dropRepeatedVerbs(parts))
}

// leadSentences opens the Summary with the dominant repo's story, then what
// the other repos did, so the Summary reads as prose rather than a count.
func leadSentences(repos []facts.RepoFacts, multi bool) string {
	d := dominantRepo(repos)
	if d == nil {
		return ""
	}
	var out []string
	lead := repoSummaryClause(d.repo, 2)
	if lead == "" {
		return ""
	}
	name := repoBaseName(d.repo.Name)
	if multi {
		out = append(out, fmt.Sprintf("Most of the activity was in %s, which %s.", name, lead))
	} else {
		out = append(out, fmt.Sprintf("Everything landed in %s, which %s.", name, lead))
	}
	var rest []string
	for _, r := range repos {
		if r.Name == d.repo.Name {
			continue
		}
		if c := repoSummaryClause(r, 1); c != "" {
			rest = append(rest, repoBaseName(r.Name)+" "+c)
		}
	}
	for i, r := range rest {
		switch {
		case i == 0:
			out = append(out, strings.Replace(r, " ", " also ", 1)+".")
		case i == 1:
			out = append(out, "Meanwhile, "+r+".")
		default:
			var names []string
			for _, o := range rest[i:] {
				names = append(names, o[:strings.Index(o, " ")])
			}
			out = append(out, "Smaller changes landed in "+joinList(names)+".")
			return strings.Join(out, " ")
		}
	}
	return strings.Join(out, " ")
}

// variationSeed derives a stable pseudo-random seed from the digest's own
// content, so phrase selection varies across days but is deterministic for a
// given day.
func variationSeed(repos []facts.RepoFacts, total int) uint32 {
	h := fnv.New32a()
	for _, r := range repos {
		h.Write([]byte(r.Name))
	}
	h.Write([]byte(strconv.Itoa(total)))
	return h.Sum32()
}

func pickVariant(pool []string, seed uint32) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[int(seed%uint32(len(pool)))]
}

// soloCommitSentence renders the whole Summary for a true single-commit day
// (one repo, one non-genesis commit) as a single varied sentence. The
// template is picked from the commit's own SHA rather than variationSeed's
// repo+total hash, so the phrasing still changes day to day even when the
// same repo lands a solo commit repeatedly — repo name and commit count
// alone would otherwise pick the same template every time.
func soloCommitSentence(best *scored) string {
	h := fnv.New32a()
	h.Write([]byte(best.repo.Name))
	h.Write([]byte(best.commit.SHA))
	tmpl := soloDayTemplates[int(h.Sum32()%uint32(len(soloDayTemplates)))]
	clause := lowerFirst(unitClause(unit{commits: []facts.CommitFacts{best.commit}}))
	return fmt.Sprintf(tmpl, repoBaseName(best.repo.Name), clause)
}

// dayCharacterization summarizes a multi-repo day's dominant commit type in
// one grounded sentence. Single-type weighting is by commit count; days with
// no recognizable dominant type get no closer.
func dayCharacterization(repos []facts.RepoFacts, seed uint32) string {
	count := map[string]int{}
	for _, r := range repos {
		for _, c := range withoutGenesis(r.Commits) {
			count[c.Type]++
		}
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	dominant := ""
	for _, k := range keys {
		if count[k] > count[dominant] {
			dominant = k
		}
	}
	return pickVariant(dayPhrases[dominant], seed)
}

// genesisCommit returns a repo's "init" commit, if it shipped one this cycle.
func genesisCommit(commits []facts.CommitFacts) *facts.CommitFacts {
	for i := range commits {
		if commits[i].Genesis {
			return &commits[i]
		}
	}
	return nil
}

func withoutGenesis(commits []facts.CommitFacts) []facts.CommitFacts {
	out := make([]facts.CommitFacts, 0, len(commits))
	for _, c := range commits {
		if !c.Genesis {
			out = append(out, c)
		}
	}
	return out
}

func genesisRepoNames(repos []facts.RepoFacts) []string {
	var out []string
	for _, r := range repos {
		if genesisCommit(r.Commits) != nil {
			out = append(out, repoBaseName(r.Name))
		}
	}
	return out
}

func breakingRepos(repos []facts.RepoFacts) []string {
	var out []string
	for _, r := range repos {
		for _, c := range r.Commits {
			if c.Breaking {
				out = append(out, repoBaseName(r.Name))
				break
			}
		}
	}
	return out
}

func sortedByScore(commits []facts.CommitFacts) []facts.CommitFacts {
	out := make([]facts.CommitFacts, len(commits))
	copy(out, commits)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// unit is one rendered clause: a lone commit or a pair of related commits
// merged into a single thematic mention. Units keep score order — the first
// commit is always the unit's highest-scored.
type unit struct {
	commits []facts.CommitFacts
}

func (u unit) notable() bool {
	for _, c := range u.commits {
		if c.Notable {
			return true
		}
	}
	return false
}

// buildUnits folds adjacent same-type commits sharing an area (or scope)
// into two-commit units, so a burst of related fixes reads as one clause
// instead of a semicolon list.
func buildUnits(sorted []facts.CommitFacts) []unit {
	units := make([]unit, 0, len(sorted))
	for _, c := range sorted {
		if !c.Notable && len(units) > 0 {
			last := &units[len(units)-1]
			prev := last.commits[len(last.commits)-1]
			if !prev.Notable && len(last.commits) < 2 && related(prev, c) {
				last.commits = append(last.commits, c)
				continue
			}
		}
		units = append(units, unit{commits: []facts.CommitFacts{c}})
	}
	return units
}

// related reports whether two commits plausibly belong to the same change:
// identical non-generic type plus a shared area. Scope alone doesn't merge —
// a repo-wide scope sweep isn't one story.
func related(a, b facts.CommitFacts) bool {
	if a.Type != b.Type || a.Type == "other" {
		return false
	}
	set := make(map[string]bool, len(a.Areas))
	for _, ar := range commitAreas(a) {
		set[ar] = true
	}
	for _, ar := range commitAreas(b) {
		if set[ar] {
			return true
		}
	}
	return false
}

// commitAreas uses the precomputed Areas when present, deriving from file
// paths otherwise, so merging never depends on upstream having filled them.
func commitAreas(c facts.CommitFacts) []string {
	if len(c.Areas) > 0 {
		return c.Areas
	}
	var out []string
	for _, f := range c.Files {
		if a := facts.AreaOf(f.Path); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func countCommits(units []unit) int {
	n := 0
	for _, u := range units {
		n += len(u.commits)
	}
	return n
}

// dominantScope returns the conventional-commit scope covering at least half
// the repo's commits (minimum two), for topic-led paragraphs.
func dominantScope(units []unit) string {
	count := map[string]int{}
	total := 0
	for _, u := range units {
		for _, c := range u.commits {
			if c.Scope != "" {
				count[c.Scope]++
			}
			total++
		}
	}
	best := ""
	for scope, n := range count {
		if n >= 2 && n*2 >= total && n > count[best] {
			best = scope
		}
	}
	return best
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// selectMentionedUnits picks the units to mention in a crowded repo
// paragraph. Notable commits (postmortems, incident reports) are guaranteed
// a slot even when their score would otherwise be crowded out — a small diff
// can still be the most important story of the day. Remaining slots fill by
// score, and the result is re-sorted by score for consistent reading order.
func selectMentionedUnits(units []unit, n int) []unit {
	if len(units) <= n {
		return units
	}
	picked := make([]bool, len(units))
	out := make([]unit, 0, n)
	for i, u := range units {
		if u.notable() && len(out) < n {
			out = append(out, u)
			picked[i] = true
		}
	}
	for i, u := range units {
		if len(out) >= n {
			break
		}
		if !picked[i] {
			out = append(out, u)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].commits[0].Score > out[j].commits[0].Score })
	return out
}

type story struct {
	units    []unit
	releases []string
}

// storyOf folds release-bump commits into a version list and the rest into
// score-ordered units.
func storyOf(commits []facts.CommitFacts) story {
	var st story
	var rest []facts.CommitFacts
	for _, c := range withoutGenesis(commits) {
		if m := releaseRe.FindStringSubmatch(strings.TrimSpace(c.Subject)); m != nil {
			st.releases = append(st.releases, m[1])
			continue
		}
		rest = append(rest, c)
	}
	st.units = buildUnits(sortedByScore(rest))
	return st
}

func (s story) releaseClause() string {
	switch len(s.releases) {
	case 0:
		return ""
	case 1:
		return "Cut release " + s.releases[0]
	}
	return "Cut releases " + joinList(s.releases)
}

var chainLeads = []string{"", "It also ", "On top of that, it ", "Finally, it "}

func chain(clauses []string) string {
	out := make([]string, 0, len(clauses))
	for i, c := range clauses {
		if i >= len(chainLeads) {
			i = len(chainLeads) - 1
		}
		if i == 0 {
			out = append(out, c+".")
			continue
		}
		out = append(out, chainLeads[i]+lowerFirst(c)+".")
	}
	return strings.Join(out, " ")
}

// dropRepeatedVerbs strips a clause's leading verb when the previous clause
// opened with the same one, so "fixed a, fixed b" reads "fixed a and b".
func dropRepeatedVerbs(clauses []string) []string {
	out := make([]string, len(clauses))
	prev := ""
	for i, c := range clauses {
		verb, rest, _ := strings.Cut(c, " ")
		if i > 0 && verb == prev && rest != "" {
			out[i] = rest
		} else {
			out[i] = c
		}
		prev = verb
	}
	return out
}

func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}

func repoParagraph(r facts.RepoFacts) string {
	var sb strings.Builder

	if g := genesisCommit(r.Commits); g != nil {
		fmt.Fprintf(&sb, "New repo: bootstrapped with an init commit (%s).", plural(len(g.Files), "file"))
		if len(withoutGenesis(r.Commits)) == 0 {
			return sb.String()
		}
		sb.WriteString(" ")
	}

	st := storyOf(r.Commits)
	if len(st.units) > maxMentioned {
		intro := fmt.Sprintf("%d commits landed here", countCommits(st.units)+len(st.releases))
		if scope := dominantScope(st.units); scope != "" {
			intro += ", most of them in " + scope
		}
		sb.WriteString(intro + ". ")
		var parts []string
		for _, u := range selectMentionedUnits(st.units, maxMentioned) {
			parts = append(parts, lowerFirst(unitClause(u)))
		}
		if rc := st.releaseClause(); rc != "" {
			parts = append(parts, lowerFirst(rc))
		}
		sb.WriteString("It " + joinList(dropRepeatedVerbs(parts)) + ".")
	} else {
		var clauses []string
		for _, u := range st.units {
			clauses = append(clauses, unitClause(u))
		}
		if rc := st.releaseClause(); rc != "" {
			clauses = append(clauses, rc)
		}
		sb.WriteString(chain(clauses))
	}

	for _, extra := range []string{statSentence(r.Commits), depSentence(r.Commits), watchSentence(r.Commits)} {
		if extra != "" {
			sb.WriteString(" " + extra)
		}
	}
	return sb.String()
}

// statSentence reports the footprint of a repo's day: files touched, lines
// added and removed, and the busiest directories.
func statSentence(commits []facts.CommitFacts) string {
	paths := map[string]bool{}
	var adds, dels int
	for _, c := range commits {
		for _, f := range c.Files {
			paths[f.Path] = true
			adds += f.Additions
			dels += f.Deletions
		}
	}
	if len(paths) == 0 {
		return ""
	}
	s := "It touched " + plural(len(paths), "file")
	switch {
	case adds > 0 && dels > 0:
		s += fmt.Sprintf(", adding %s and removing %d", plural(adds, "line"), dels)
	case adds > 0:
		s += ", adding " + plural(adds, "line")
	case dels > 0:
		s += ", removing " + plural(dels, "line")
	}
	areaCount := map[string]int{}
	for p := range paths {
		if a := facts.AreaOf(p); a != "" {
			areaCount[a]++
		}
	}
	areas := make([]string, 0, len(areaCount))
	for a := range areaCount {
		areas = append(areas, a)
	}
	sort.Slice(areas, func(i, j int) bool {
		if areaCount[areas[i]] != areaCount[areas[j]] {
			return areaCount[areas[i]] > areaCount[areas[j]]
		}
		return areas[i] < areas[j]
	})
	if len(areas) > 2 {
		areas = areas[:2]
	}
	if len(areas) > 0 {
		s += ", mostly in " + joinList(areas)
	}
	return s + "."
}

// firstWord returns the past-tense verb for a commit type.
func firstWord(typ string, verbs map[string]string) string {
	if v, ok := verbs[typ]; ok && v != "" {
		return v
	}
	return verbs["other"]
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// pastOf returns the past tense of a recognized imperative verb, guessing
// only for unambiguous -ize/-ify forms; "" when w is not a verb.
func pastOf(w string) string {
	lw := strings.ToLower(w)
	if p, ok := imperativePast[lw]; ok {
		return p
	}
	if pastForm[lw] {
		return lw
	}
	switch {
	case len(lw) >= 7 && strings.HasSuffix(lw, "ize"):
		return lw + "d"
	case len(lw) >= 6 && strings.HasSuffix(lw, "ify"):
		return strings.TrimSuffix(lw, "y") + "ied"
	}
	return ""
}

func fixAcronyms(s string) string {
	words := strings.Split(s, " ")
	for i, w := range words {
		core := strings.TrimRight(w, ",.;:)!?")
		if a, ok := acronyms[core]; ok {
			words[i] = a + w[len(core):]
		}
	}
	return strings.Join(words, " ")
}

// commitLead splits a commit into the past-tense verb that opens its clause
// and the remaining body. A recognized leading verb in the subject becomes
// the clause verb itself; otherwise the commit type supplies one.
func commitLead(c facts.CommitFacts) (string, string) {
	segs := strings.Split(strings.TrimSpace(c.Subject), ", ")
	words := strings.Fields(segs[0])
	verb := firstWord(c.Type, verbPast)
	if len(words) > 1 && !notVerbFollowers[strings.ToLower(words[1])] {
		if past := pastOf(words[0]); past != "" {
			verb = capFirst(past)
			words = words[1:]
		}
	}
	body := lowerLead(strings.Join(words, " "))
	for _, seg := range segs[1:] {
		body += ", " + conjugateSegment(seg)
	}
	if strings.TrimSpace(body) == "" {
		body = "a change"
	}
	return verb, fixAcronyms(body)
}

func lowerLead(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 || acronymLike(words[0]) {
		return s
	}
	return lowerFirst(s)
}

// unitClause renders one unit as a capitalized clause: the verb, each
// commit's body joined with "and", and shared evidence.
func unitClause(u unit) string {
	verb, body := commitLead(u.commits[0])
	parts := []string{body}
	for _, c := range u.commits[1:] {
		v, b := commitLead(c)
		if v == verb {
			parts = append(parts, b)
		} else {
			parts = append(parts, lowerFirst(v)+" "+b)
		}
	}
	text := verb + " " + strings.Join(parts, " and ")
	if ev := evidence(u.commits); ev != "" {
		text += " " + ev
	}
	return text
}

// conjugateSegment rewrites a comma-separated subject segment so its verb
// reads as past tense ("dampen data-dump churn" -> "dampened data-dump churn").
func conjugateSegment(seg string) string {
	words := strings.Fields(strings.TrimSpace(seg))
	if len(words) == 0 {
		return seg
	}
	if past := pastOf(words[0]); past != "" {
		words[0] = past
	} else if !acronymLike(words[0]) {
		head := []rune(words[0])
		head[0] = unicode.ToLower(head[0])
		words[0] = string(head)
	}
	return strings.Join(words, " ")
}

var acronymLike = func(w string) bool {
	r := []rune(w)
	return len(r) > 1 && unicode.IsUpper(r[1])
}

var (
	goFlagRe  = regexp.MustCompile(`flag\.\w+(?:Var)?\("([^"]+)"`)
	longOptRe = regexp.MustCompile(`(?:^|[\s"'=(])--([a-z][a-z0-9][a-z0-9-]*)\b`)
	routeRe   = regexp.MustCompile(`["'` + "`" + `](/[a-z0-9][\w./:-]{1,48})["'` + "`" + `]`)
)

// evidence cites the user-facing surface of a change: CLI flags and route
// paths mined from added lines. Function names and directory areas are
// deliberately not cited — identifiers read as robotic.
func evidence(commits []facts.CommitFacts) string {
	var flags, routes []string
	for _, t := range minedTokens(commits) {
		if strings.HasPrefix(t, "-") {
			flags = append(flags, t)
		} else {
			routes = append(routes, t)
		}
	}
	var parts []string
	if len(flags) > 0 {
		parts = append(parts, "the "+joinList(flags)+" "+pluralWord(len(flags), "flag"))
	}
	if len(routes) > 0 {
		parts = append(parts, "the "+joinList(routes)+" "+pluralWord(len(routes), "route"))
	}
	if len(parts) == 0 {
		return ""
	}
	return "around " + joinList(parts)
}

func pluralWord(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// minedTokens extracts flag names and route paths from added lines — the
// user-facing surface of a change, which reads better than internal symbols.
func minedTokens(commits []facts.CommitFacts) []string {
	seen := map[string]bool{}
	var subjects []string
	for _, c := range commits {
		subjects = append(subjects, strings.ToLower(c.Subject))
	}
	var out []string
	add := func(tok string) {
		if tok == "" || seen[tok] || len(tok) < 2 || strings.Contains(tok, " ") {
			return
		}
		for _, s := range subjects {
			if strings.Contains(s, strings.ToLower(strings.TrimLeft(tok, "-"))) {
				return // already named in the subject — don't cite twice
			}
		}
		seen[tok] = true
		out = append(out, tok)
	}
	for _, c := range commits {
		for _, f := range c.Files {
			for _, line := range f.AddedLines {
				for _, m := range goFlagRe.FindAllStringSubmatch(line, -1) {
					add("-" + m[1])
				}
				for _, m := range longOptRe.FindAllStringSubmatch(line, -1) {
					add("--" + m[1])
				}
				for _, m := range routeRe.FindAllStringSubmatch(line, -1) {
					add(m[1])
				}
			}
		}
	}
	if len(out) > 2 {
		out = out[:2]
	}
	return out
}

func depSentence(commits []facts.CommitFacts) string {
	var parts []string
	seen := map[string]bool{}
	for _, c := range commits {
		for _, d := range c.Deps {
			key := d.Ecosystem + "/" + d.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			switch d.Action {
			case "bumped":
				p := fmt.Sprintf("bumped %s from %s to %s", d.Name, d.Old, d.New)
				if d.Major {
					p += " (a major jump)"
				}
				parts = append(parts, p)
			case "added":
				parts = append(parts, fmt.Sprintf("added %s %s", d.Name, d.New))
			case "removed":
				parts = append(parts, "removed "+d.Name)
			case "downgraded":
				parts = append(parts, fmt.Sprintf("downgraded %s from %s to %s", d.Name, d.Old, d.New))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 3 {
		parts = append(parts[:3], fmt.Sprintf("%d more", len(parts)-3))
	}
	return "On the dependency side, it " + joinList(parts) + "."
}

func watchSentence(commits []facts.CommitFacts) string {
	flagSet := map[string]bool{}
	breaking := false
	for _, c := range commits {
		for _, f := range c.RiskFlags {
			flagSet[f] = true
		}
		breaking = breaking || c.Breaking
	}
	if len(flagSet) == 0 && !breaking {
		return ""
	}
	var phrases []string
	for f := range flagSet {
		if p, ok := riskPhrases[f]; ok {
			phrases = append(phrases, p)
		} else {
			phrases = append(phrases, f)
		}
	}
	sort.Strings(phrases)
	var clauses []string
	if len(phrases) > 0 {
		clauses = append(clauses, "touches "+joinList(phrases))
	}
	if breaking {
		clauses = append(clauses, "includes breaking changes")
	}
	return "Heads-up: it " + joinList(clauses) + "."
}

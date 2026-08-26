// Package render stitches extracted facts into digest markdown with plain
// templates. Every phrase traces back to an input fact — the renderer cannot
// invent filenames, functions, versions, or behaviors.
package render

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/facts"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/symbols"
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

var leadingVerb = map[string]map[string]bool{
	"feat": {"add": true, "adds": true, "added": true, "introduce": true,
		"introduced": true, "implement": true, "implemented": true,
		"support": true, "supported": true, "enable": true, "enabled": true},
	"fix": {"fix": true, "fixes": true, "fixed": true, "repair": true,
		"handle": true, "handled": true, "resolve": true, "resolved": true},
	"refactor": {"refactor": true, "refactored": true, "rename": true,
		"renamed": true, "move": true, "moved": true, "extract": true},
	"perf": {"speed": true, "optimize": true, "optimized": true, "memoize": true},
	"docs": {"document": true, "documented": true, "add": true, "update": true},
	"test": {"test": true, "tests": true, "add": true},
	"chore": {"bump": true, "bumped": true, "update": true, "updated": true,
		"upgrade": true, "tidy": true, "clean": true, "cleaned": true},
	"build": {"bump": true, "bumped": true, "update": true, "updated": true},
	"ci":    {"update": true, "updated": true, "add": true},
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
	sb.WriteString(summaryLine(total, len(repos)))
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
	if best := biggestChange(repos); best != nil {
		sb.WriteString(fmt.Sprintf(" Biggest change in %s: %s.",
			best.repo.Name, strings.ToLower(firstWord(best.commit.Type, verbPast))+clauseBody(best.commit)))
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Per-Repo Activity\n")
	for _, r := range repos {
		sb.WriteString("\n### " + r.Name + "\n\n")
		sb.WriteString(repoParagraph(r))
		sb.WriteString("\n")
	}
	return sb.String()
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
}

func biggestChange(repos []facts.RepoFacts) *scored {
	var best *scored
	for _, r := range repos {
		for _, c := range r.Commits {
			if c.Genesis {
				continue // surfaced separately as a new-repo launch
			}
			if best == nil || c.Score > best.commit.Score {
				bc := c
				br := r
				best = &scored{repo: br, commit: bc}
			}
		}
	}
	return best
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
			out = append(out, r.Name)
		}
	}
	return out
}

func breakingRepos(repos []facts.RepoFacts) []string {
	var out []string
	for _, r := range repos {
		for _, c := range r.Commits {
			if c.Breaking {
				out = append(out, r.Name)
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

// selectMentioned picks the n commits to mention in a repo paragraph.
// Notable commits (postmortems, incident reports) are guaranteed a slot even
// when their score would otherwise be crowded out by higher-churn commits —
// a small diff can still be the most important story of the day. Remaining
// slots fill by score, and the result is re-sorted by score for a consistent
// reading order.
func selectMentioned(sorted []facts.CommitFacts, n int) []facts.CommitFacts {
	if len(sorted) <= n {
		return sorted
	}
	picked := make([]bool, len(sorted))
	out := make([]facts.CommitFacts, 0, n)
	for i, c := range sorted {
		if c.Notable && len(out) < n {
			out = append(out, c)
			picked[i] = true
		}
	}
	for i, c := range sorted {
		if len(out) >= n {
			break
		}
		if !picked[i] {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func repoParagraph(r facts.RepoFacts) string {
	var sb strings.Builder
	commits := r.Commits

	if g := genesisCommit(commits); g != nil {
		fmt.Fprintf(&sb, "New repo: bootstrapped with an init commit (%s).", plural(len(g.Files), "file"))
		commits = withoutGenesis(commits)
		if len(commits) == 0 {
			return sb.String()
		}
		sb.WriteString(" ")
	}

	sorted := sortedByScore(commits)

	if len(sorted) > maxMentioned {
		fmt.Fprintf(&sb, "Shipped %d commits: ", len(sorted))
		mentioned := selectMentioned(sorted, maxMentioned)
		clauses := make([]string, 0, len(mentioned))
		for _, c := range mentioned {
			clauses = append(clauses, strings.ToLower(firstWord(c.Type, verbPast))+clauseBody(c))
		}
		sb.WriteString(strings.Join(clauses, "; "))
		sb.WriteString(".")
	} else {
		sentences := make([]string, 0, len(sorted))
		for _, c := range sorted {
			sentences = append(sentences, firstWord(c.Type, verbPast)+clauseBody(c)+".")
		}
		sb.WriteString(strings.Join(sentences, " "))
	}

	if deps := depSentence(r.Commits); deps != "" {
		sb.WriteString(" " + deps)
	}
	if w := watchSentence(r.Commits); w != "" {
		sb.WriteString(" " + w)
	}
	return sb.String()
}

// firstWord returns the past-tense verb for a commit type.
func firstWord(typ string, verbs map[string]string) string {
	if v, ok := verbs[typ]; ok && v != "" {
		return v
	}
	return verbs["other"]
}

func clauseBody(c facts.CommitFacts) string {
	subj := cleanSubject(c.Subject, c.Type)
	ev := evidence(c)
	if ev != "" {
		return " " + subj + " " + ev
	}
	return " " + subj
}

var acronymLike = func(w string) bool {
	r := []rune(w)
	return len(r) > 1 && unicode.IsUpper(r[1])
}

// cleanSubject strips a subject's leading verb when it duplicates the template
// verb ("Add rate limiting" under feat), and lowercases the first rune unless
// it looks like an acronym (JWTAuth).
func cleanSubject(subj, typ string) string {
	words := strings.Fields(strings.TrimSpace(subj))
	if len(words) == 0 {
		return ""
	}
	first := strings.ToLower(words[0])
	stripped := words
	if set := leadingVerb[typ]; set[first] {
		stripped = words[1:]
	}
	if len(stripped) == 0 {
		return ""
	}
	head := []rune(stripped[0])
	if !acronymLike(string(head)) {
		head[0] = unicode.ToLower(head[0])
	}
	stripped[0] = string(head)
	return strings.Join(stripped, " ")
}

// evidence picks up to two changed-function names, falling back to areas.
func evidence(c facts.CommitFacts) string {
	seen := map[string]bool{}
	var fns []string
	for _, f := range c.Files {
		for _, ctx := range f.FuncContexts {
			name := symbols.CleanFuncName(ctx)
			if name != "" && !seen[name] {
				seen[name] = true
				fns = append(fns, name)
			}
		}
	}
	items := fns
	if len(items) == 0 {
		items = c.Areas
	}
	if len(items) > 2 {
		items = items[:2]
	}
	if len(items) == 0 {
		return ""
	}
	return "(" + strings.Join(items, ", ") + ")"
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
				p := fmt.Sprintf("bumped %s %s -> %s", d.Name, d.Old, d.New)
				if d.Major {
					p += " (major)"
				}
				parts = append(parts, p)
			case "added":
				parts = append(parts, fmt.Sprintf("added %s %s", d.Name, d.New))
			case "removed":
				parts = append(parts, "removed "+d.Name)
			case "downgraded":
				parts = append(parts, fmt.Sprintf("downgraded %s %s -> %s", d.Name, d.Old, d.New))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 3 {
		parts = append(parts[:3], fmt.Sprintf("+%d more", len(parts)-3))
	}
	return "Dependency updates: " + strings.Join(parts, ", ") + "."
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
	var flags []string
	for f := range flagSet {
		flags = append(flags, f)
	}
	sort.Strings(flags)
	s := "Watch:"
	if len(flags) > 0 {
		s += " " + strings.Join(flags, ", ") + ";"
	}
	if breaking {
		s += " includes breaking changes."
	} else {
		s = strings.TrimSuffix(s, ";") + "."
	}
	return s
}

// Package staticdigest renders git-digest activity JSON into a grounded
// markdown digest with deterministic templates — no LLM anywhere in the loop.
package staticdigest

import (
	"io"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/pipeline"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/render"
)

// Generate reads git-digest's repoActivity JSON schema from r and returns the
// rendered markdown digest ("## Summary" + "## Per-Repo Activity"). note, when
// set, is reproduced verbatim as the Summary's opening sentence.
func Generate(r io.Reader, note string) (string, error) {
	repos, err := pipeline.Build(r)
	if err != nil {
		return "", err
	}
	return render.Digest(repos, note), nil
}

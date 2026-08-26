// Package staticdigest renders git-digest activity JSON into a grounded
// markdown digest with deterministic templates — no LLM anywhere in the loop.
package staticdigest

import (
	"io"

	"github.com/usr-wwelsh/git-digest/internal/staticdigest/pipeline"
	"github.com/usr-wwelsh/git-digest/internal/staticdigest/render"
)

// Generate reads git-digest's repoActivity JSON schema from r and returns the
// rendered markdown digest ("## Summary" + "## Per-Repo Activity").
func Generate(r io.Reader) (string, error) {
	repos, err := pipeline.Build(r)
	if err != nil {
		return "", err
	}
	return render.Digest(repos), nil
}

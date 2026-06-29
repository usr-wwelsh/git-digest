package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func pushToPortfolio(mdPath string) error {
	portfolioPath := os.Getenv("GIT_DIGEST_PORTFOLIO")
	if portfolioPath == "" {
		return fmt.Errorf("GIT_DIGEST_PORTFOLIO not set")
	}

	destDir := filepath.Join(portfolioPath, "digests")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	dest := filepath.Join(destDir, filepath.Base(mdPath))
	if err := copyFile(mdPath, dest); err != nil {
		return err
	}

	commitsPath := strings.TrimSuffix(mdPath, ".md") + "-commits.json"
	if _, err := os.Stat(commitsPath); err == nil {
		commitsDest := filepath.Join(destDir, filepath.Base(commitsPath))
		if err := copyFile(commitsPath, commitsDest); err != nil {
			return err
		}
	}

	fmt.Printf("Copied digest to %s\n", destDir)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

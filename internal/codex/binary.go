package codex

import (
	"os"
	"os/exec"
	"path/filepath"
)

func BinaryPath() (string, error) {
	if configured := os.Getenv("CPQ_CODEX_BIN"); configured != "" {
		return configured, nil
	}
	if configured := os.Getenv("CODEX_BIN"); configured != "" {
		return configured, nil
	}
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}

	for _, candidate := range codexBinaryCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}

	return "", exec.ErrNotFound
}

func codexBinaryCandidates() []string {
	candidates := []string{
		"/usr/local/bin/codex",
		"/usr/bin/codex",
		"/opt/homebrew/bin/codex",
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return candidates
	}

	candidates = append(candidates,
		filepath.Join(home, ".local", "bin", "codex"),
		filepath.Join(home, ".npm-global", "bin", "codex"),
		filepath.Join(home, ".volta", "bin", "codex"),
		filepath.Join(home, ".bun", "bin", "codex"),
	)

	if matches, err := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "codex")); err == nil {
		candidates = append(matches, candidates...)
	}

	return candidates
}

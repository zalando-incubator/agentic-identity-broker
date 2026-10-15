package config

import (
	"os"
	"path/filepath"
)

const (
	configDirName  = "aibctl"
	configFileName = "config.yaml"
)

// DefaultPath returns the OS-specific per-user configuration file. It follows
// os.UserConfigDir and falls back to $HOME/.config when that is unavailable.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, configDirName, configFileName), nil
}

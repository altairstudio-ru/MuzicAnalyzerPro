package library

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	defaultBasePath = "~/.muzicanalyzer"
	configFilename  = "config.yaml"
	dbFilename      = "library.db"
	audioDirName    = "audio"
)

// Config holds the application configuration.
type Config struct {
	Suno SunoConfig `yaml:"suno"`
}

// SunoConfig holds Suno-specific configuration.
type SunoConfig struct {
	AuthToken string `yaml:"auth_token"`
	BasePath  string `yaml:"base_path"`
}

// LoadConfig loads configuration from the config file.
// If the config file doesn't exist, returns default config.
func LoadConfig(basePath string) (*Config, error) {
	if basePath == "" {
		basePath = defaultBasePath
	}
	basePath = expandPath(basePath)

	configPath := filepath.Join(basePath, configFilename)
	cfg := &Config{
		Suno: SunoConfig{
			BasePath: basePath,
		},
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // return default config
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Ensure BasePath is set
	if cfg.Suno.BasePath == "" {
		cfg.Suno.BasePath = basePath
	}

	return cfg, nil
}

// SaveConfig writes the configuration to disk.
func SaveConfig(cfg *Config) error {
	basePath := expandPath(cfg.Suno.BasePath)
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	configPath := filepath.Join(basePath, configFilename)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// DBDir returns the path to the database file.
func (cfg *Config) DBDir() string {
	return expandPath(cfg.Suno.BasePath)
}

// DBPath returns the path to the SQLite database file.
func (cfg *Config) DBPath() string {
	return filepath.Join(cfg.DBDir(), dbFilename)
}

// AudioDir returns the path to the audio storage directory.
func (cfg *Config) AudioDir() string {
	return filepath.Join(expandPath(cfg.Suno.BasePath), audioDirName)
}

// WorkspaceAudioDir returns the path for a specific workspace's audio files.
func (cfg *Config) WorkspaceAudioDir(workspace string) string {
	return filepath.Join(cfg.AudioDir(), sanitizeDirName(workspace))
}

// expandPath replaces "~" with the user's home directory.
func expandPath(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + path[1:]
	}
	return path
}

// sanitizeDirName replaces characters unsafe for directory names.
func sanitizeDirName(name string) string {
	// Replace common unsafe characters
	result := make([]byte, 0, len(name))
	for _, c := range []byte(name) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c >= '0' && c <= '9' || c == '-' || c == '_' || c == ' ' || c == '.' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}

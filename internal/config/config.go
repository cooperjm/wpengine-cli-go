package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the WP Engine CLI configurations.
type Config struct {
	Username         string `yaml:"username"`
	Password         string `yaml:"password"`
	AccountID        string `yaml:"account_id"`
	SSHKeyPath       string `yaml:"ssh_key_path"`
	SSHKeyPassphrase string `yaml:"ssh_key_passphrase"`
	BatchConcurrency int    `yaml:"batch_concurrency"`
	Interactive      bool   `yaml:"interactive"`
}

const ConfigFileName = ".wpengine-cli.yaml"

var configPathOverride string

// SetConfigPath overrides the default config path for the current process.
func SetConfigPath(path string) {
	configPathOverride = path
}

// GetConfigPath returns the path to the configuration file.
func GetConfigPath() (string, error) {
	if configPathOverride != "" {
		return configPathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigFileName), nil
}

// Load loads the configuration from the file.
func Load() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		BatchConcurrency: 10,
		Interactive:      true,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file does not exist
			return cfg, nil
		}
		return nil, err
	}

	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, err
	}

	// Expand default SSH key path if empty
	if cfg.SSHKeyPath == "" {
		home, _ := os.UserHomeDir()
		// Try to find id_ed25519 first, then id_rsa
		ed25519Path := filepath.Join(home, ".ssh", "id_ed25519")
		if _, err := os.Stat(ed25519Path); err == nil {
			cfg.SSHKeyPath = ed25519Path
		} else {
			cfg.SSHKeyPath = filepath.Join(home, ".ssh", "id_rsa")
		}
	}

	return cfg, nil
}

// Save saves the configuration to the file.
func (c *Config) Save() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Ensure parent directory exists (though it is home, so it should)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

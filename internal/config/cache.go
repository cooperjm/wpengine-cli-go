package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"wpengine-cli/internal/api"
)

const CacheFileName = ".wpengine-cli-cache.json"

// GetCachePath returns the path to the cache file in the user's home directory.
func GetCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, CacheFileName), nil
}

// LoadCache loads the cached environments from the cache file.
func LoadCache() ([]api.Install, error) {
	path, err := GetCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var installs []api.Install
	if err := json.Unmarshal(data, &installs); err != nil {
		return nil, err
	}

	return installs, nil
}

// SaveCache saves the list of environments to the cache file.
func SaveCache(installs []api.Install) error {
	path, err := GetCachePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(installs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// ResolveFromCache searches for an environment by name or ID in the local cache.
func ResolveFromCache(target string) (*api.Install, error) {
	installs, err := LoadCache()
	if err != nil {
		return nil, err
	}

	for _, inst := range installs {
		if strings.EqualFold(inst.Name, target) || strings.EqualFold(inst.ID, target) {
			return &inst, nil
		}
	}

	return nil, nil
}

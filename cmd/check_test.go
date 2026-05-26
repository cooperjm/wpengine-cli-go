package cmd

import (
	"errors"
	"path/filepath"
	"testing"

	"wpengine-cli/internal/ui"
)

func TestRenderSummaryTable(t *testing.T) {
	results := []SiteCheckResult{
		{
			EnvName: "prod-env",
			CoreNeed: []CoreUpdateInfo{
				{Version: "6.2", UpdateType: "minor"},
			},
			PluginsNeed: []PluginUpdateInfo{
				{Name: "akismet", Version: "5.0", UpdateVersion: "5.1", Status: "active"},
				{Name: "jetpack", Version: "11.0", UpdateVersion: "11.1", Status: "inactive"},
			},
			ThemesNeed: []ThemeUpdateInfo{
				{Name: "twentytwentythree", Version: "1.0", UpdateVersion: "1.1", Status: "active"},
			},
			Err: nil,
		},
		{
			EnvName:     "clean-env",
			CoreNeed:    nil,
			PluginsNeed: nil,
			ThemesNeed:  nil,
			Err:         nil,
		},
		{
			EnvName:     "error-env",
			CoreNeed:    nil,
			PluginsNeed: nil,
			ThemesNeed:  nil,
			Err:         errors.New("SSH connection failed"),
		},
	}

	// Invoke renderSummaryTable to ensure it doesn't panic and prints correct table representation
	renderSummaryTable(results)
}

func TestCheckCachingAndHook(t *testing.T) {
	// Temporarily redirect check results path to a temp file
	tempDir := t.TempDir()
	originalGetCachePath := getCheckResultsCachePath
	getCheckResultsCachePath = func() (string, error) {
		return filepath.Join(tempDir, "test-check-results.json"), nil
	}
	defer func() {
		getCheckResultsCachePath = originalGetCachePath
	}()

	results := []SiteCheckResult{
		{
			EnvName: "test-site-1",
			CoreNeed: []CoreUpdateInfo{
				{Version: "6.2", UpdateType: "minor"},
			},
			PluginsNeed: []PluginUpdateInfo{
				{Name: "akismet", Version: "5.0", UpdateVersion: "5.1", Status: "active"},
			},
			ThemesNeed: []ThemeUpdateInfo{
				{Name: "twentytwentythree", Version: "1.0", UpdateVersion: "1.1", Status: "active"},
			},
			Err: nil,
		},
		{
			EnvName: "test-site-2",
			Err:     errors.New("SSH error"),
		},
	}

	// 1. Test saveCheckResults
	err := saveCheckResults(results)
	if err != nil {
		t.Fatalf("failed to save check results: %v", err)
	}

	// 2. Test loadCheckResults
	cached, err := loadCheckResults()
	if err != nil {
		t.Fatalf("failed to load check results: %v", err)
	}
	if cached == nil || len(cached.Results) != 2 {
		t.Fatalf("expected 2 cached results, got %v", cached)
	}
	if cached.Results[0].EnvName != "test-site-1" || cached.Results[0].CoreNeed[0].Version != "6.2" {
		t.Errorf("cached result values mismatch: %+v", cached.Results[0])
	}
	if cached.Results[1].ErrorStr != "SSH error" {
		t.Errorf("cached result error mismatch: %s", cached.Results[1].ErrorStr)
	}

	// 3. Test updateCachedCheckResults
	// Mock a successful plugin-only update for test-site-1
	completedJobs := []*ui.Job{
		{
			Name:   "test-site-1",
			Status: "completed",
		},
		{
			Name:   "test-site-2",
			Status: "failed", // failed jobs shouldn't clear errors
		},
	}

	updateCachedCheckResults(completedJobs, "plugins")

	// Reload and verify
	cached, err = loadCheckResults()
	if err != nil {
		t.Fatalf("failed to reload check results: %v", err)
	}

	// PluginsNeed should be empty, but CoreNeed and ThemesNeed should remain
	if len(cached.Results[0].PluginsNeed) != 0 {
		t.Errorf("expected plugins to be cleared in cache, got %+v", cached.Results[0].PluginsNeed)
	}
	if len(cached.Results[0].CoreNeed) != 1 {
		t.Errorf("expected core update to remain in cache")
	}
	if len(cached.Results[0].ThemesNeed) != 1 {
		t.Errorf("expected theme update to remain in cache")
	}
	if cached.Results[1].ErrorStr != "SSH error" {
		t.Errorf("expected failed job error to remain in cache")
	}

	// Mock a successful all-update for test-site-1 and a successful connection retry for test-site-2
	completedJobs = []*ui.Job{
		{
			Name:   "test-site-1",
			Status: "completed",
		},
		{
			Name:   "test-site-2",
			Status: "completed", // this time succeeded (e.g. core update)
		},
	}

	updateCachedCheckResults(completedJobs, "all")

	// Reload and verify
	cached, err = loadCheckResults()
	if err != nil {
		t.Fatalf("failed to reload check results: %v", err)
	}

	if len(cached.Results[0].CoreNeed) != 0 || len(cached.Results[0].ThemesNeed) != 0 {
		t.Errorf("expected all updates to be cleared for test-site-1")
	}
	if cached.Results[1].ErrorStr != "" {
		t.Errorf("expected error to be cleared for test-site-2 after successful update")
	}
}

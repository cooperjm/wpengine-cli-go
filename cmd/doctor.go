package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ux"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

var doctorRefreshCache bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check local setup, API access, SSH key, cache, and terminal compatibility",
	Example: `  wpengine doctor
  wpengine doctor --refresh-cache
  wpengine doctor --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		checks := runDoctorChecks()
		if OutputFormat == "json" {
			data, err := json.MarshalIndent(checks, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			if doctorHasFailure(checks) {
				return fmt.Errorf("doctor found setup problems")
			}
			return nil
		}

		fmt.Println()
		fmt.Println(ux.Style(PrimaryStyle, "WP Engine CLI Doctor", PlainOutput))
		for _, check := range checks {
			color := lipgloss.Color("46")
			label := "OK"
			if check.Status == "warn" {
				color = lipgloss.Color("214")
				label = "WARN"
			}
			if check.Status == "fail" {
				color = lipgloss.Color("196")
				label = "FAIL"
			}
			fmt.Printf("%s %-18s %s\n", ux.Badge(label, color, PlainOutput), check.Name, check.Message)
		}
		fmt.Println()

		passed, warned, failed := doctorCounts(checks)
		if PlainOutput {
			fmt.Printf("%d passed · %d warnings · %d failures\n\n", passed, warned, failed)
		} else {
			passedStr := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(fmt.Sprintf("%d passed", passed))
			warnStr := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(fmt.Sprintf("%d warnings", warned))
			failStr := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("%d failures", failed))
			fmt.Printf("%s · %s · %s\n\n", passedStr, warnStr, failStr)
		}

		if doctorHasFailure(checks) {
			return fmt.Errorf("doctor found setup problems")
		}
		return nil
	},
}

func doctorCounts(checks []doctorCheck) (passed, warned, failed int) {
	for _, c := range checks {
		switch c.Status {
		case "ok":
			passed++
		case "warn":
			warned++
		case "fail":
			failed++
		}
	}
	return
}

func doctorHasFailure(checks []doctorCheck) bool {
	for _, check := range checks {
		if check.Status == "fail" {
			return true
		}
	}
	return false
}

func runDoctorChecks() []doctorCheck {
	var checks []doctorCheck

	configPath, err := config.GetConfigPath()
	if err != nil {
		checks = append(checks, doctorCheck{"config", "fail", err.Error()})
	} else if _, err := os.Stat(configPath); err == nil {
		checks = append(checks, doctorCheck{"config", "ok", configPath})
	} else {
		checks = append(checks, doctorCheck{"config", "warn", "not found; run wpengine config configure"})
	}

	if Cfg.Username != "" && Cfg.Password != "" {
		checks = append(checks, doctorCheck{"credentials", "ok", "API username and password are configured"})
	} else {
		checks = append(checks, doctorCheck{"credentials", "fail", "missing API credentials; run wpengine config configure"})
	}

	if APIClient != nil {
		accounts, err := APIClient.GetAccounts(1, 0)
		if err != nil {
			checks = append(checks, doctorCheck{"api", "fail", err.Error()})
		} else if len(accounts.Results) == 0 {
			checks = append(checks, doctorCheck{"api", "warn", "credentials worked, but no accounts were returned"})
		} else {
			checks = append(checks, doctorCheck{"api", "ok", fmt.Sprintf("authenticated; first account: %s", accounts.Results[0].Name)})
		}
	} else {
		checks = append(checks, doctorCheck{"api", "fail", "API client was not created because credentials are missing"})
	}

	if Cfg.AccountID != "" {
		checks = append(checks, doctorCheck{"account", "ok", Cfg.AccountID})
	} else {
		checks = append(checks, doctorCheck{"account", "warn", "default account is not set; some create flows will prompt or fall back"})
	}

	if Cfg.SSHKeyPath == "" {
		checks = append(checks, doctorCheck{"ssh key", "warn", "no SSH key path configured"})
	} else if _, err := os.Stat(Cfg.SSHKeyPath); err == nil {
		checks = append(checks, doctorCheck{"ssh key", "ok", Cfg.SSHKeyPath})
	} else {
		checks = append(checks, doctorCheck{"ssh key", "fail", fmt.Sprintf("%s is not readable", Cfg.SSHKeyPath)})
	}

	cachePath, err := config.GetCachePath()
	if err != nil {
		checks = append(checks, doctorCheck{"cache", "warn", err.Error()})
	} else {
		installs, err := config.LoadCache()
		if err == nil && len(installs) > 0 {
			checks = append(checks, doctorCheck{"cache", "ok", fmt.Sprintf("%d environments cached at %s", len(installs), cachePath)})
		} else if doctorRefreshCache && APIClient != nil {
			installs, err := APIClient.GetAllInstalls()
			if err != nil {
				checks = append(checks, doctorCheck{"cache", "fail", fmt.Sprintf("refresh failed: %v", err)})
			} else if err := config.SaveCache(installs); err != nil {
				checks = append(checks, doctorCheck{"cache", "fail", fmt.Sprintf("save failed: %v", err)})
			} else {
				checks = append(checks, doctorCheck{"cache", "ok", fmt.Sprintf("refreshed %d environments at %s", len(installs), cachePath)})
			}
		} else {
			checks = append(checks, doctorCheck{"cache", "warn", "empty or missing; run wpengine site list or wpengine doctor --refresh-cache"})
		}
	}

	if ux.Plain(PlainOutput) {
		checks = append(checks, doctorCheck{"terminal", "warn", "plain output mode is active"})
	} else if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		checks = append(checks, doctorCheck{"terminal", "warn", "TERM=dumb; use --plain for stable output"})
	} else {
		checks = append(checks, doctorCheck{"terminal", "ok", "styled terminal output is available"})
	}

	return checks
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorRefreshCache, "refresh-cache", false, "fetch environments and rebuild the local cache")
	RootCmd.AddCommand(doctorCmd)
}

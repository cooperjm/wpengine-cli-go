package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"wpengine-cli/internal/api"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	checkAllEnvs    bool
	checkBatch      string
	checkConcurrent int
)

// JSON structures for WP-CLI command output parsing
type CoreUpdateInfo struct {
	Version    string `json:"version"`
	UpdateType string `json:"update_type"`
}

type PluginUpdateInfo struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	UpdateVersion string `json:"update_version"`
}

type ThemeUpdateInfo struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	UpdateVersion string `json:"update_version"`
}

// SiteCheckResult stores the check status for a specific site
type SiteCheckResult struct {
	EnvName    string
	CoreNeed   []CoreUpdateInfo
	PluginsNeed []PluginUpdateInfo
	ThemesNeed  []ThemeUpdateInfo
	Err        error
}

var checkCmd = &cobra.Command{
	Use:   "check [install_name_or_id]",
	Short: "Check for WordPress core, plugin, and theme updates",
	Long: `Securely connects via SSH to the targeted environments and queries WP-CLI to 
identify available updates for WordPress core, plugins, and themes.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return RequireAPI()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var targets []string
		if len(args) > 0 {
			targets = append(targets, args[0])
		}

		if checkBatch != "" {
			if _, err := os.Stat(checkBatch); err == nil {
				file, err := os.Open(checkBatch)
				if err != nil {
					return fmt.Errorf("failed to open batch file: %w", err)
				}
				defer file.Close()
				// Read line by line
				var lines []string
				var line string
				for {
					_, err := fmt.Fscanln(file, &line)
					if err != nil {
						break
					}
					trimmed := strings.TrimSpace(line)
					if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
						lines = append(lines, trimmed)
					}
				}
				targets = append(targets, lines...)
			} else {
				parts := strings.Split(checkBatch, ",")
				for _, p := range parts {
					trimmed := strings.TrimSpace(p)
					if trimmed != "" {
						targets = append(targets, trimmed)
					}
				}
			}
		}

		installsResp, err := APIClient.GetInstalls(100, 0)
		if err != nil {
			return fmt.Errorf("failed to fetch environments: %w", err)
		}

		if checkAllEnvs {
			for _, inst := range installsResp.Results {
				if inst.Status == "active" {
					targets = append(targets, inst.Name)
				}
			}
		}

		if len(targets) == 0 {
			return fmt.Errorf("no check targets specified. Provide an environment name, or use --batch or --all-envs")
		}

		// Concurrency limit
		concurrency := checkConcurrent
		if concurrency <= 0 {
			concurrency = Cfg.BatchConcurrency
		}
		if concurrency <= 0 {
			concurrency = 3
		}

		// Run checks
		runChecks(targets, installsResp.Results, concurrency)
		return nil
	},
}

func runChecks(targets []string, installs []api.Install, concurrency int) {
	fmt.Println("\n" + lipgloss.NewStyle().Background(lipgloss.Color("39")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" CHECKING ") + " Scanning environments for available updates...")
	fmt.Printf("Targets: %d | Concurrency: %d\n\n", len(targets), concurrency)

	// Resolve names
	type targetInfo struct {
		name string
		id   string
	}
	var resolved []targetInfo
	for _, target := range targets {
		found := false
		for _, inst := range installs {
			if strings.EqualFold(inst.Name, target) || strings.EqualFold(inst.ID, target) {
				resolved = append(resolved, targetInfo{name: inst.Name, id: inst.ID})
				found = true
				break
			}
		}
		if !found {
			// Print error directly for missing ones
			badge := lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" ERROR ")
			fmt.Printf("%s %s: Environment not found or inactive in account\n", badge, target)
		}
	}

	if len(resolved) == 0 {
		return
	}

	jobChan := make(chan targetInfo, len(resolved))
	for _, r := range resolved {
		jobChan <- r
	}
	close(jobChan)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var printMu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobChan {
				sem <- struct{}{}
				
				res := checkEnvironmentUpdates(r.name)
				
				printMu.Lock()
				renderCheckResult(res)
				printMu.Unlock()

				<-sem
			}
		}()
	}

	wg.Wait()
}

func checkEnvironmentUpdates(envName string) SiteCheckResult {
	res := SiteCheckResult{EnvName: envName}

	// Verify connection first
	if err := SSHClient.VerifyConnection(envName); err != nil {
		res.Err = fmt.Errorf("SSH connection failed: %w", err)
		return res
	}

	// 1. Check Core Updates
	stdout, _, err := SSHClient.RunWPCLI(envName, "core", "check-update", "--format=json")
	if err == nil && stdout != "" && stdout != "[]" {
		var cores []CoreUpdateInfo
		if err := json.Unmarshal([]byte(stdout), &cores); err == nil {
			res.CoreNeed = cores
		}
	}

	// 2. Check Plugin Updates
	stdout, _, err = SSHClient.RunWPCLI(envName, "plugin", "list", "--update=available", "--format=json")
	if err == nil && stdout != "" && stdout != "[]" {
		var plugins []PluginUpdateInfo
		if err := json.Unmarshal([]byte(stdout), &plugins); err == nil {
			res.PluginsNeed = plugins
		}
	}

	// 3. Check Theme Updates
	stdout, _, err = SSHClient.RunWPCLI(envName, "theme", "list", "--update=available", "--format=json")
	if err == nil && stdout != "" && stdout != "[]" {
		var themes []ThemeUpdateInfo
		if err := json.Unmarshal([]byte(stdout), &themes); err == nil {
			res.ThemesNeed = themes
		}
	}

	return res
}

func renderCheckResult(res SiteCheckResult) {
	primaryCol := lipgloss.Color("99")
	titleStyle := lipgloss.NewStyle().Foreground(primaryCol).Bold(true)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryCol).
		Padding(1, 2).
		Margin(1, 0)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(fmt.Sprintf("Update Report for: %s", res.EnvName)) + "\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n")

	if res.Err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		sb.WriteString(errStyle.Render(fmt.Sprintf("Error checking updates: %v", res.Err)) + "\n")
		fmt.Println(box.Render(sb.String()))
		return
	}

	hasUpdates := false

	// WordPress Core
	coreTitle := lipgloss.NewStyle().Bold(true).Render("WordPress Core:")
	if len(res.CoreNeed) > 0 {
		badge := lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(" UPDATE AVAILABLE ")
		sb.WriteString(fmt.Sprintf("%s %s Target: Version %s\n", coreTitle, badge, res.CoreNeed[0].Version))
		hasUpdates = true
	} else {
		badge := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render("Up-to-date")
		sb.WriteString(fmt.Sprintf("%s %s\n", coreTitle, badge))
	}

	sb.WriteString("\n")

	// Plugins
	pluginTitle := lipgloss.NewStyle().Bold(true).Render("Plugins:")
	if len(res.PluginsNeed) > 0 {
		badge := lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(fmt.Sprintf(" %d UPDATES AVAILABLE ", len(res.PluginsNeed)))
		sb.WriteString(fmt.Sprintf("%s %s\n", pluginTitle, badge))
		for _, p := range res.PluginsNeed {
			statusTag := ""
			if p.Status == "active" {
				statusTag = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(" [active]")
			}
			sb.WriteString(fmt.Sprintf("  • %s (%s -> %s)%s\n", p.Name, p.Version, p.UpdateVersion, statusTag))
		}
		hasUpdates = true
	} else {
		badge := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render("All plugins up-to-date")
		sb.WriteString(fmt.Sprintf("%s %s\n", pluginTitle, badge))
	}

	sb.WriteString("\n")

	// Themes
	themeTitle := lipgloss.NewStyle().Bold(true).Render("Themes:")
	if len(res.ThemesNeed) > 0 {
		badge := lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(fmt.Sprintf(" %d UPDATES AVAILABLE ", len(res.ThemesNeed)))
		sb.WriteString(fmt.Sprintf("%s %s\n", themeTitle, badge))
		for _, t := range res.ThemesNeed {
			statusTag := ""
			if t.Status == "active" {
				statusTag = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(" [active]")
			}
			sb.WriteString(fmt.Sprintf("  • %s (%s -> %s)%s\n", t.Name, t.Version, t.UpdateVersion, statusTag))
		}
		hasUpdates = true
	} else {
		badge := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render("All themes up-to-date")
		sb.WriteString(fmt.Sprintf("%s %s\n", themeTitle, badge))
	}

	sb.WriteString("\n")

	if !hasUpdates {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render("✔ Website is fully up-to-date!") + "\n")
	}

	fmt.Println(box.Render(sb.String()))
}

func init() {
	checkCmd.Flags().StringVar(&checkBatch, "batch", "", "Comma-separated list of environment names, or path to a text file with targets")
	checkCmd.Flags().BoolVar(&checkAllEnvs, "all-envs", false, "Check all active environments under the account")
	checkCmd.Flags().IntVar(&checkConcurrent, "concurrency", 0, "Concurrency limit for checks (falls back to config batch_concurrency)")

	RootCmd.AddCommand(checkCmd)
}

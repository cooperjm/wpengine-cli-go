package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
)

type targetInfo struct {
	name    string
	id      string
	envType string
}

var (
	checkAllEnvs    bool
	checkBatch      string
	checkConcurrent int
	checkMinimal    bool
	checkVerbose    bool // new
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
// SiteCheckResult stores the check status for a specific site
type SiteCheckResult struct {
	EnvName     string
	EnvType     string
	CoreNeed    []CoreUpdateInfo
	PluginsNeed []PluginUpdateInfo
	ThemesNeed  []ThemeUpdateInfo
	Err         error
}

type CachedSiteCheckResult struct {
	EnvName     string             `json:"env_name"`
	EnvType     string             `json:"env_type"`
	CoreNeed    []CoreUpdateInfo   `json:"core_need"`
	PluginsNeed []PluginUpdateInfo `json:"plugins_need"`
	ThemesNeed  []ThemeUpdateInfo  `json:"themes_need"`
	ErrorStr    string             `json:"error_str"`
}

type CachedCheckResults struct {
	Timestamp time.Time               `json:"timestamp"`
	Results   []CachedSiteCheckResult `json:"results"`
}

var getCheckResultsCachePath = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".wpengine-cli-check-results.json"), nil
}

func saveCheckResults(results []SiteCheckResult) error {
	path, err := getCheckResultsCachePath()
	if err != nil {
		return err
	}

	cachedResults := make([]CachedSiteCheckResult, len(results))
	for i, r := range results {
		var errStr string
		if r.Err != nil {
			errStr = r.Err.Error()
		}
		cachedResults[i] = CachedSiteCheckResult{
			EnvName:     r.EnvName,
			EnvType:     r.EnvType,
			CoreNeed:    r.CoreNeed,
			PluginsNeed: r.PluginsNeed,
			ThemesNeed:  r.ThemesNeed,
			ErrorStr:    errStr,
		}
	}

	data := CachedCheckResults{
		Timestamp: time.Now(),
		Results:   cachedResults,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, jsonData, 0600)
}

func loadCheckResults() (*CachedCheckResults, error) {
	path, err := getCheckResultsCachePath()
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

	var cached CachedCheckResults
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	return &cached, nil
}

func updateCachedCheckResults(completedJobs []*ui.Job, scope string) {
	cached, err := loadCheckResults()
	if err != nil || cached == nil || len(cached.Results) == 0 {
		return
	}

	updatedAny := false
	for _, job := range completedJobs {
		if job.Status != "completed" {
			continue
		}

		// Find in cache
		for idx, cr := range cached.Results {
			if strings.EqualFold(cr.EnvName, job.Name) {
				// Clear the updated items in cache
				if scope == "core" || scope == "all" {
					cached.Results[idx].CoreNeed = nil
				}
				if scope == "plugins" || scope == "all" {
					cached.Results[idx].PluginsNeed = nil
				}
				if scope == "themes" || scope == "all" {
					cached.Results[idx].ThemesNeed = nil
				}

				// Clear error if it succeeded now
				cached.Results[idx].ErrorStr = ""
				updatedAny = true
				break
			}
		}
	}

	if updatedAny {
		// Convert CachedSiteCheckResult back to SiteCheckResult to save
		results := make([]SiteCheckResult, len(cached.Results))
		for i, cr := range cached.Results {
			var rErr error
			if cr.ErrorStr != "" {
				rErr = errors.New(cr.ErrorStr)
			}
			results[i] = SiteCheckResult{
				EnvName:     cr.EnvName,
				EnvType:     cr.EnvType,
				CoreNeed:    cr.CoreNeed,
				PluginsNeed: cr.PluginsNeed,
				ThemesNeed:  cr.ThemesNeed,
				Err:         rErr,
			}
		}

		_ = saveCheckResults(results)
	}
}

var checkCmd = &cobra.Command{
	Use:   "check [install_name_or_id]",
	Short: "Check for WordPress core, plugin, and theme updates",
	Long: `Securely connects via SSH to the targeted environments and queries WP-CLI to 
identify available updates for WordPress core, plugins, and themes.`,
	Example: `  wpengine check my-dev-sandbox
  wpengine check --all-envs --verbose
  wpengine check --batch target_envs.txt --output json
  wpengine check`,
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

		// 3. Resolve Active Environments
		var installs []api.Install
		var err error

		if checkAllEnvs {
			installs, err = APIClient.GetAllInstalls()
			if err != nil {
				return fmt.Errorf("failed to fetch environments: %w", err)
			}
			_ = config.SaveCache(installs)

			for _, inst := range installs {
				if inst.Status == "active" {
					targets = append(targets, inst.Name)
				}
			}
		}

		if len(targets) == 0 {
			cached, err := loadCheckResults()
			if err == nil && cached != nil && len(cached.Results) > 0 {
				results := make([]SiteCheckResult, len(cached.Results))
				for i, cr := range cached.Results {
					var rErr error
					if cr.ErrorStr != "" {
						rErr = errors.New(cr.ErrorStr)
					}
					results[i] = SiteCheckResult{
						EnvName:     cr.EnvName,
						EnvType:     cr.EnvType,
						CoreNeed:    cr.CoreNeed,
						PluginsNeed: cr.PluginsNeed,
						ThemesNeed:  cr.ThemesNeed,
						Err:         rErr,
					}
				}

				timeStr := cached.Timestamp.Format("Jan 02, 2006 at 3:04 PM")
				if OutputFormat == "json" {
					return printCheckResultsJSON(results, cached.Timestamp, true)
				}
				fmt.Println("\n" + lipgloss.NewStyle().Background(ui.PrimaryColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" CACHED RESULTS ") + " Displaying results from check run on " + timeStr)
				fmt.Println(lipgloss.NewStyle().Foreground(ui.MutedColor).Render("Run 'wpengine check --all-envs' (or with specific targets) to scan for live updates.") + "\n")

				renderSummaryTable(results)

				// Determine if there are any environments with pending updates
				var pendingUpdates []siteSelectionItem
				for _, r := range results {
					if r.Err != nil {
						continue
					}
					if len(r.CoreNeed) > 0 || len(r.PluginsNeed) > 0 || len(r.ThemesNeed) > 0 {
						var parts []string
						if len(r.CoreNeed) > 0 {
							parts = append(parts, "WordPress Core")
						}
						if len(r.PluginsNeed) > 0 {
							parts = append(parts, fmt.Sprintf("%d plugin(s)", len(r.PluginsNeed)))
						}
						if len(r.ThemesNeed) > 0 {
							parts = append(parts, fmt.Sprintf("%d theme(s)", len(r.ThemesNeed)))
						}
						pendingUpdates = append(pendingUpdates, siteSelectionItem{
							envName:     r.EnvName,
							envType:     r.EnvType,
							updatesDesc: strings.Join(parts, ", "),
							selected:    false,
						})
					}
				}

				if len(pendingUpdates) > 0 {
					selModel := siteSelectionModel{items: pendingUpdates}
					p := tea.NewProgram(selModel)
					finalModel, err := p.Run()
					if err == nil {
						m := finalModel.(siteSelectionModel)
						if m.chosen {
							var toUpdate []string
							for _, item := range m.items {
								if item.selected {
									toUpdate = append(toUpdate, item.envName)
								}
							}

							if len(toUpdate) > 0 {
								fmt.Printf("Starting updates for: %s\n", strings.Join(toUpdate, ", "))
								err := runUpdatesForEnvironments(toUpdate, "all", Cfg.BatchConcurrency, "", false)
								if err != nil {
									return err
								}
							}
						}
					}
				}

				return nil
			}

			return fmt.Errorf("no check targets specified and no cached results found. Provide an environment name, or use --batch or --all-envs")
		}

		// Concurrency limit
		concurrency := checkConcurrent
		if concurrency <= 0 {
			concurrency = Cfg.BatchConcurrency
		}
		if concurrency <= 0 {
			concurrency = 10
		}

		// Resolve names to installs
		var resolved []targetInfo
		for _, target := range targets {
			// Try resolving from cache first
			cachedInst, _ := config.ResolveFromCache(target)
			if cachedInst != nil {
				resolved = append(resolved, targetInfo{name: cachedInst.Name, id: cachedInst.ID, envType: shortEnvType(cachedInst.Environment)})
				continue
			}

			// Fetch from API if not cached (fallback)
			if len(installs) == 0 {
				installs, err = APIClient.GetAllInstalls()
				if err != nil {
					return fmt.Errorf("failed to fetch environments: %w", err)
				}
				_ = config.SaveCache(installs)
			}

			found := false
			for _, inst := range installs {
				if strings.EqualFold(inst.Name, target) || strings.EqualFold(inst.ID, target) {
					resolved = append(resolved, targetInfo{name: inst.Name, id: inst.ID, envType: shortEnvType(inst.Environment)})
					found = true
					break
				}
			}
			if !found {
				// Print error directly for missing ones
				badge := lipgloss.NewStyle().Background(ui.ErrorColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" ERROR ")
				fmt.Printf("%s %s: Environment not found or inactive in account\n", badge, target)
			}
		}

		if len(resolved) == 0 {
			return nil
		}

		// Run checks
		results := runChecks(resolved, concurrency)
		if OutputFormat == "json" {
			return printCheckResultsJSON(results, time.Now(), false)
		}
		return nil
	},
}

type checkJob struct {
	target targetInfo
	index  int
}

func runChecks(resolved []targetInfo, concurrency int) []SiteCheckResult {
	if OutputFormat != "json" {
		fmt.Println("\n" + lipgloss.NewStyle().Background(ui.InfoColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" CHECKING ") + " Scanning environments for available updates...")
		fmt.Printf("Targets: %d | Concurrency: %d\n\n", len(resolved), concurrency)
	}

	jobChan := make(chan checkJob, len(resolved))
	for idx, r := range resolved {
		jobChan <- checkJob{target: r, index: idx}
	}
	close(jobChan)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var printMu sync.Mutex
	results := make([]SiteCheckResult, len(resolved))

	var completedCount int
	var countMu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobChan {
				sem <- struct{}{}

				res := checkEnvironmentUpdates(j.target.name, j.target.envType)
				results[j.index] = res

				if OutputFormat == "json" {
					// Keep JSON output clean for automation.
				} else if checkVerbose {
					printMu.Lock()
					renderCheckResult(res)
					printMu.Unlock()
				} else {
					countMu.Lock()
					completedCount++
					count := completedCount
					countMu.Unlock()
					fmt.Printf("\r\033[K%s Scanned %d/%d environments...",
						lipgloss.NewStyle().Foreground(ui.PrimaryColor).Bold(true).Render("●"),
						count, len(resolved))
				}

				<-sem
			}
		}()
	}

	wg.Wait()

	if OutputFormat != "json" {
		if !checkVerbose {
			fmt.Print("\r\033[K") // clear the progress line
		}
		renderSummaryTable(results)
	}

	_ = saveCheckResults(results)
	return results
}

func printCheckResultsJSON(results []SiteCheckResult, timestamp time.Time, cached bool) error {
	type checkResult struct {
		EnvName     string             `json:"env_name"`
		EnvType     string             `json:"env_type"`
		CoreNeed    []CoreUpdateInfo   `json:"core_need,omitempty"`
		PluginsNeed []PluginUpdateInfo `json:"plugins_need,omitempty"`
		ThemesNeed  []ThemeUpdateInfo  `json:"themes_need,omitempty"`
		Error       string             `json:"error,omitempty"`
	}
	payload := struct {
		Cached    bool          `json:"cached"`
		Timestamp time.Time     `json:"timestamp"`
		Results   []checkResult `json:"results"`
	}{Cached: cached, Timestamp: timestamp}

	for _, result := range results {
		item := checkResult{
			EnvName:     result.EnvName,
			EnvType:     result.EnvType,
			CoreNeed:    result.CoreNeed,
			PluginsNeed: result.PluginsNeed,
			ThemesNeed:  result.ThemesNeed,
		}
		if result.Err != nil {
			item.Error = result.Err.Error()
		}
		payload.Results = append(payload.Results, item)
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func renderSummaryTable(results []SiteCheckResult) {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ui.PrimaryColor)).
		Headers("Environment", "Type", "WordPress", "Plugins", "Themes", "Status")

	t.StyleFunc(func(row, col int) lipgloss.Style {
		if row == table.HeaderRow {
			return lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(ui.PrimaryColor).
				Bold(true).
				Padding(0, 1)
		}
		return lipgloss.NewStyle().Padding(0, 1)
	})

	for _, res := range results {
		var wpStr, pluginStr, themeStr, statusStr string

		envStr := lipgloss.NewStyle().Bold(true).Render(res.EnvName)

		var typeBadge string
		switch strings.ToLower(res.EnvType) {
		case "prod":
			typeBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Bold(true).Render("PROD")
		case "stg":
			typeBadge = lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true).Render("STG")
		case "dev":
			typeBadge = lipgloss.NewStyle().Foreground(ui.InfoColor).Bold(true).Render("DEV")
		default:
			if res.EnvType != "" {
				typeBadge = strings.ToUpper(res.EnvType)
			} else {
				typeBadge = lipgloss.NewStyle().Foreground(ui.MutedColor).Bold(true).Render("—")
			}
		}

		if res.Err != nil {
			wpStr = "—"
			pluginStr = "—"
			themeStr = "—"
			statusStr = lipgloss.NewStyle().Foreground(ui.ErrorColor).Bold(true).Render(fmt.Sprintf("Error: %v", res.Err))
		} else {
			// WordPress core
			if len(res.CoreNeed) > 0 {
				wpStr = lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true).Render(fmt.Sprintf("Update Available (%s)", res.CoreNeed[0].Version))
			} else {
				wpStr = lipgloss.NewStyle().Foreground(ui.SuccessColor).Render("Up-to-date")
			}

			// Plugins
			if len(res.PluginsNeed) > 0 {
				pluginStr = lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true).Render(fmt.Sprintf("%d updates", len(res.PluginsNeed)))
			} else {
				pluginStr = lipgloss.NewStyle().Foreground(ui.SuccessColor).Render("0")
			}

			// Themes
			if len(res.ThemesNeed) > 0 {
				themeStr = lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true).Render(fmt.Sprintf("%d updates", len(res.ThemesNeed)))
			} else {
				themeStr = lipgloss.NewStyle().Foreground(ui.SuccessColor).Render("0")
			}

			// Overall status
			if len(res.CoreNeed) > 0 || len(res.PluginsNeed) > 0 || len(res.ThemesNeed) > 0 {
				statusStr = lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true).Render("Updates pending")
			} else {
				statusStr = lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("Up-to-date")
			}
		}

		t.Row(envStr, typeBadge, wpStr, pluginStr, themeStr, statusStr)
	}

	fmt.Println(t.Render())

	// Print summary metrics
	summaryStyle := lipgloss.NewStyle().Bold(true)
	greenStyle := lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true)
	orangeStyle := lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true)
	redStyle := lipgloss.NewStyle().Foreground(ui.ErrorColor).Bold(true)

	var totalEnvs, upToDate, pending, errorsCount int
	for _, res := range results {
		totalEnvs++
		if res.Err != nil {
			errorsCount++
		} else if len(res.CoreNeed) > 0 || len(res.PluginsNeed) > 0 || len(res.ThemesNeed) > 0 {
			pending++
		} else {
			upToDate++
		}
	}

	fmt.Printf("%s %d environments | %s | %s | %s\n\n",
		summaryStyle.Render("Summary:"),
		totalEnvs,
		greenStyle.Render(fmt.Sprintf("%d Up-to-date", upToDate)),
		orangeStyle.Render(fmt.Sprintf("%d Updates pending", pending)),
		redStyle.Render(fmt.Sprintf("%d Errors", errorsCount)),
	)
}

func checkEnvironmentUpdates(envName string, envType string) SiteCheckResult {
	res := SiteCheckResult{EnvName: envName, EnvType: envType}

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
	primaryCol := ui.PrimaryColor
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
		errStyle := lipgloss.NewStyle().Foreground(ui.ErrorColor).Bold(true)
		sb.WriteString(errStyle.Render(fmt.Sprintf("Error checking updates: %v", res.Err)) + "\n")
		fmt.Println(box.Render(sb.String()))
		return
	}

	hasUpdates := false

	// WordPress Core
	coreTitle := lipgloss.NewStyle().Bold(true).Render("WordPress Core:")
	if len(res.CoreNeed) > 0 {
		badge := lipgloss.NewStyle().Background(ui.WarningColor).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(" UPDATE AVAILABLE ")
		sb.WriteString(fmt.Sprintf("%s %s Target: Version %s\n", coreTitle, badge, res.CoreNeed[0].Version))
		hasUpdates = true
	} else {
		badge := lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("Up-to-date")
		sb.WriteString(fmt.Sprintf("%s %s\n", coreTitle, badge))
	}

	sb.WriteString("\n")

	// Plugins
	pluginTitle := lipgloss.NewStyle().Bold(true).Render("Plugins:")
	if len(res.PluginsNeed) > 0 {
		badge := lipgloss.NewStyle().Background(ui.WarningColor).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(fmt.Sprintf(" %d UPDATES AVAILABLE ", len(res.PluginsNeed)))
		sb.WriteString(fmt.Sprintf("%s %s\n", pluginTitle, badge))
		for _, p := range res.PluginsNeed {
			statusTag := ""
			if p.Status == "active" {
				statusTag = lipgloss.NewStyle().Foreground(ui.SuccessColor).Render(" [active]")
			}
			sb.WriteString(fmt.Sprintf("  • %s (%s -> %s)%s\n", p.Name, p.Version, p.UpdateVersion, statusTag))
		}
		hasUpdates = true
	} else {
		badge := lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("All plugins up-to-date")
		sb.WriteString(fmt.Sprintf("%s %s\n", pluginTitle, badge))
	}

	sb.WriteString("\n")

	// Themes
	themeTitle := lipgloss.NewStyle().Bold(true).Render("Themes:")
	if len(res.ThemesNeed) > 0 {
		badge := lipgloss.NewStyle().Background(ui.WarningColor).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(fmt.Sprintf(" %d UPDATES AVAILABLE ", len(res.ThemesNeed)))
		sb.WriteString(fmt.Sprintf("%s %s\n", themeTitle, badge))
		for _, t := range res.ThemesNeed {
			statusTag := ""
			if t.Status == "active" {
				statusTag = lipgloss.NewStyle().Foreground(ui.SuccessColor).Render(" [active]")
			}
			sb.WriteString(fmt.Sprintf("  • %s (%s -> %s)%s\n", t.Name, t.Version, t.UpdateVersion, statusTag))
		}
		hasUpdates = true
	} else {
		badge := lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("All themes up-to-date")
		sb.WriteString(fmt.Sprintf("%s %s\n", themeTitle, badge))
	}

	sb.WriteString("\n")

	if !hasUpdates {
		sb.WriteString(lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("✔ Website is fully up-to-date!") + "\n")
	}

	fmt.Println(box.Render(sb.String()))
}

func init() {
	checkCmd.Flags().StringVar(&checkBatch, "batch", "", "Comma-separated list of environment names, or path to a text file with targets")
	checkCmd.Flags().BoolVar(&checkAllEnvs, "all-envs", false, "Check all active environments under the account")
	checkCmd.Flags().IntVar(&checkConcurrent, "concurrency", 0, "Concurrency limit for checks (falls back to config batch_concurrency)")
	checkCmd.Flags().BoolVar(&checkVerbose, "verbose", false, "Show detailed per-environment update report for each result")
	checkCmd.Flags().BoolVarP(&checkMinimal, "minimal", "m", false, "") // deprecated, now default behavior
	checkCmd.Flags().MarkHidden("minimal")

	RootCmd.AddCommand(checkCmd)
}

func shortEnvType(longType string) string {
	switch strings.ToLower(longType) {
	case "production":
		return "prod"
	case "staging":
		return "stg"
	case "development":
		return "dev"
	default:
		return longType
	}
}

type siteSelectionItem struct {
	envName     string
	envType     string
	updatesDesc string
	selected    bool
}

type siteSelectionModel struct {
	items    []siteSelectionItem
	cursor   int
	chosen   bool
	quitting bool
}

func (m siteSelectionModel) Init() tea.Cmd {
	return nil
}

func (m siteSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case " ": // Toggle selection
			m.items[m.cursor].selected = !m.items[m.cursor].selected

		case "a": // Select all
			for i := range m.items {
				m.items[i].selected = true
			}

		case "n": // Select none
			for i := range m.items {
				m.items[i].selected = false
			}

		case "enter": // Confirm
			m.chosen = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m siteSelectionModel) View() string {
	if m.quitting {
		return "\nSelection cancelled.\n"
	}
	if m.chosen {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(ui.PrimaryColor).Bold(true).Render("Select environments to update:") + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(ui.MutedColor).Render("Use Up/Down or j/k to navigate, [Space] to toggle, [a] select all, [n] select none, [Enter] to run updates, [q] to cancel.") + "\n\n")

	maxVisible := 10
	start := 0
	end := len(m.items)

	if len(m.items) > maxVisible {
		start = m.cursor - maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + maxVisible
		if end > len(m.items) {
			end = len(m.items)
			start = end - maxVisible
		}
	}

	if start > 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(ui.MutedColor).Render(fmt.Sprintf("  ▲ ... %d more environments above ...", start)) + "\n")
	}

	for i := start; i < end; i++ {
		item := m.items[i]
		cursorStr := "  "
		var itemStyle lipgloss.Style
		if m.cursor == i {
			cursorStr = lipgloss.NewStyle().Foreground(ui.PrimaryColor).Render("➔ ")
			itemStyle = lipgloss.NewStyle().Foreground(ui.PrimaryColor).Bold(true)
		} else {
			itemStyle = lipgloss.NewStyle()
		}

		checkStr := "[ ]"
		if item.selected {
			checkStr = lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("[✔]")
		}

		typeBadge := ""
		switch strings.ToLower(item.envType) {
		case "prod":
			typeBadge = lipgloss.NewStyle().Background(lipgloss.Color("160")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" PROD ")
		case "stg":
			typeBadge = lipgloss.NewStyle().Background(ui.WarningColor).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(" STG  ")
		case "dev":
			typeBadge = lipgloss.NewStyle().Background(ui.InfoColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" DEV  ")
		default:
			if item.envType != "" {
				typeBadge = lipgloss.NewStyle().Background(ui.MutedColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" " + strings.ToUpper(item.envType) + " ")
			} else {
				typeBadge = lipgloss.NewStyle().Background(ui.MutedColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" UNK  ")
			}
		}

		nameStr := itemStyle.Render(item.envName)
		descStr := lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(item.updatesDesc)

		sb.WriteString(fmt.Sprintf("%s%s  %s  %s  (%s)\n", cursorStr, checkStr, typeBadge, nameStr, descStr))
	}

	if end < len(m.items) {
		sb.WriteString(lipgloss.NewStyle().Foreground(ui.MutedColor).Render(fmt.Sprintf("  ▼ ... %d more environments below ...", len(m.items)-end)) + "\n")
	}

	// Show currently selected count
	selectedCount := 0
	for _, item := range m.items {
		if item.selected {
			selectedCount++
		}
	}
	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(ui.PrimaryColor).Bold(true).Render(fmt.Sprintf("Selected: %d environments", selectedCount)) + "\n")

	return sb.String()
}

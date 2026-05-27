package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ui"
	"wpengine-cli/internal/ux"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
)

var (
	updateAll        bool
	updatePlugins    bool
	updateThemes     bool
	updateCore       bool
	updateDryRun     bool
	updateBatch      string
	updateAllEnvs    bool
	updateConcurrent int
	updateEmail      string
)

var updateCmd = &cobra.Command{
	Use:   "update [install_name_or_id]",
	Short: "Run backups and updates on one or more environments",
	Long: `Triggers a backup checkpoint on the targeted environments, polls until the backup 
completes, and then securely connects via SSH to run WordPress updates using WP-CLI.`,
	Example: `  wpengine update my-dev-sandbox --plugins --dry-run
  wpengine update my-production --all --email admin@example.com
  wpengine update --batch target_envs.txt --themes --no-interactive --yes
  wpengine update --all-envs --all --output json --yes`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return RequireAPI()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Determine Update Scope
		scope := "plugins" // default
		if updateAll {
			scope = "all"
		} else if updateCore {
			scope = "core"
		} else if updateThemes {
			scope = "themes"
		} else if updatePlugins {
			scope = "plugins"
		}

		// 2. Collect Environment Targets
		var targets []string
		if len(args) > 0 {
			targets = append(targets, args[0])
		}

		if updateBatch != "" {
			// Read batch names from comma-separated string or file
			if _, err := os.Stat(updateBatch); err == nil {
				// Read from file
				file, err := os.Open(updateBatch)
				if err != nil {
					return fmt.Errorf("failed to open batch file: %w", err)
				}
				defer file.Close()
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line != "" && !strings.HasPrefix(line, "#") {
						targets = append(targets, line)
					}
				}
			} else {
				// Comma-separated list
				parts := strings.Split(updateBatch, ",")
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

		if updateAllEnvs {
			installs, err = APIClient.GetAllInstalls()
			if err != nil {
				return fmt.Errorf("failed to fetch environments from API: %w", err)
			}
			_ = config.SaveCache(installs)

			for _, inst := range installs {
				if inst.Status == "active" {
					targets = append(targets, inst.Name)
				}
			}
		}

		if len(targets) == 0 {
			return fmt.Errorf("no update targets specified. Provide an environment name, or use --batch or --all-envs")
		}

		// Resolve names to install objects
		return runUpdatesForEnvironments(targets, scope, updateConcurrent, updateEmail, updateDryRun)
	},
}

func runUpdatesForEnvironments(targets []string, scope string, concurrency int, email string, dryRun bool) error {
	var installs []api.Install
	var err error

	// Resolve names to install objects
	var resolvedJobs []*ui.Job
	for _, target := range targets {
		// First try resolving from cache
		cachedInst, _ := config.ResolveFromCache(target)
		if cachedInst != nil {
			resolvedJobs = append(resolvedJobs, &ui.Job{
				ID:      cachedInst.ID,
				Name:    cachedInst.Name,
				EnvType: cachedInst.Environment,
				Status:  "idle",
			})
			continue
		}

		// If not found in cache, fetch all installs from API (fallback)
		if len(installs) == 0 {
			installs, err = APIClient.GetAllInstalls()
			if err != nil {
				return fmt.Errorf("failed to fetch environments from API: %w", err)
			}
			_ = config.SaveCache(installs)
		}

		found := false
		for _, inst := range installs {
			if strings.EqualFold(inst.Name, target) || strings.EqualFold(inst.ID, target) {
				resolvedJobs = append(resolvedJobs, &ui.Job{
					ID:      inst.ID,
					Name:    inst.Name,
					EnvType: inst.Environment,
					Status:  "idle",
				})
				found = true
				break
			}
		}

		if !found {
			// Add anyway, but status failed
			resolvedJobs = append(resolvedJobs, &ui.Job{
				Name:   target,
				Status: "failed",
				Error:  fmt.Errorf("environment not found or inactive in account"),
			})
		}
	}

	// Determine concurrency
	if concurrency <= 0 {
		concurrency = Cfg.BatchConcurrency
	}
	if concurrency <= 0 {
		concurrency = 10
	}

	if err := confirmUpdatePlan(resolvedJobs, scope, concurrency, email, dryRun); err != nil {
		return err
	}

	// Run Execution Loop
	// Select interactive TUI or static log mode
	interactive := Cfg.Interactive && !NoInter && OutputFormat != "json" && !ux.Plain(PlainOutput)
	if interactive {
		m := ui.NewModel(resolvedJobs, APIClient, SSHClient, scope, dryRun, concurrency, email)
		p := tea.NewProgram(m)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}
	} else {
		runNonInteractive(resolvedJobs, scope, concurrency)
	}

	// Update Check Results Cache if not a dry-run
	if !dryRun {
		updateCachedCheckResults(resolvedJobs, scope)
	}

	if OutputFormat == "json" {
		if err := printUpdateResultsJSON(resolvedJobs, scope, concurrency, dryRun); err != nil {
			return err
		}
	}

	return nil
}

func confirmUpdatePlan(jobs []*ui.Job, scope string, concurrency int, email string, dryRun bool) error {
	if dryRun || AssumeYes {
		return nil
	}

	var productionCount, runnableCount int
	for _, job := range jobs {
		if job.Status == "failed" {
			continue
		}
		runnableCount++
		if strings.EqualFold(job.EnvType, "production") || strings.EqualFold(job.EnvType, "prod") {
			productionCount++
		}
	}

	if runnableCount == 0 {
		return nil
	}
	if runnableCount == 1 && productionCount == 0 {
		return nil
	}

	emailDesc := email
	if emailDesc == "" {
		emailDesc = "no-reply@wpengine.com"
	}
	fmt.Println()
	fmt.Println(ux.Badge("REVIEW", ui.WarningColor, PlainOutput) + " Update plan")
	fmt.Printf("Targets: %d | Production: %d | Scope: %s | Concurrency: %d | Backup email: %s\n", runnableCount, productionCount, scope, concurrency, emailDesc)
	fmt.Println("A backup checkpoint will be created before each update.")

	ok, err := ux.Confirm("Proceed with this update plan?", AssumeYes)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("update cancelled")
	}
	return nil
}

func printUpdateResultsJSON(jobs []*ui.Job, scope string, concurrency int, dryRun bool) error {
	type jobResult struct {
		Name    string `json:"name"`
		ID      string `json:"id,omitempty"`
		EnvType string `json:"environment_type,omitempty"`
		Status  string `json:"status"`
		Details string `json:"details,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	payload := struct {
		Scope       string      `json:"scope"`
		Concurrency int         `json:"concurrency"`
		DryRun      bool        `json:"dry_run"`
		Results     []jobResult `json:"results"`
	}{Scope: scope, Concurrency: concurrency, DryRun: dryRun}

	for _, job := range jobs {
		result := jobResult{
			Name:    job.Name,
			ID:      job.ID,
			EnvType: job.EnvType,
			Status:  job.Status,
			Details: strings.TrimSpace(job.Details),
		}
		if job.Error != nil {
			result.Error = job.Error.Error()
		}
		payload.Results = append(payload.Results, result)
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// runNonInteractive executes the updates using clean Lipgloss-styled print statements (useful for scripting/CI).
func runNonInteractive(jobs []*ui.Job, scope string, concurrency int) {
	if OutputFormat != "json" {
		fmt.Println("\n" + ui.GetStatusBadge("updating") + " Starting Batch Updates (Non-Interactive Mode)")
		fmt.Printf("Targets: %d | Concurrency: %d | Scope: %s\n\n", len(jobs), concurrency, scope)
	}

	jobChan := make(chan int, len(jobs))
	for i := range jobs {
		jobChan <- i
	}
	close(jobChan)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var logMu sync.Mutex
	printLog := func(badge, name, message string, color lipgloss.Color) {
		if OutputFormat == "json" {
			return
		}
		logMu.Lock()
		defer logMu.Unlock()
		ui.PrintLog(badge, name, message, color)
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobChan {
				sem <- struct{}{}

				job := jobs[idx]
				if job.Status == "failed" {
					printLog("FAILED", job.Name, job.Error.Error(), ui.ErrorColor)
					<-sem
					continue
				}

				// SSH Verify
				printLog("VERIFY", job.Name, "Testing SSH gateway connection...", ui.InfoColor)
				err := SSHClient.VerifyConnection(job.Name)
				if err != nil {
					printLog("FAILED", job.Name, fmt.Sprintf("SSH connection failed: %v", err), ui.ErrorColor)
					job.Status = "failed"
					job.Error = err
					<-sem
					continue
				}

				// Trigger Backup
				printLog("BACKUP", job.Name, "Creating backup checkpoint...", ui.WarningColor)

				backupDesc := fmt.Sprintf("cli-pre-update-%d", time.Now().Unix())
				var emails []string
				if updateEmail != "" {
					emails = []string{updateEmail}
				}
				backup, err := APIClient.CreateBackup(job.ID, backupDesc, emails)
				if err != nil {
					printLog("FAILED", job.Name, fmt.Sprintf("Backup failed: %v", err), ui.ErrorColor)
					job.Status = "failed"
					job.Error = err
					<-sem
					continue
				}

				// Poll Backup
				statusChan, errChan := APIClient.PollBackupStatus(job.ID, backup.ID, 8*time.Second, 15*time.Minute)
				backupSuccess := true
			pollLoop:
				for {
					select {
					case b, ok := <-statusChan:
						if !ok {
							break pollLoop
						}
						printLog("BACKUP", job.Name, fmt.Sprintf("Polling status: %s", b.Status), ui.WarningColor)
					case err := <-errChan:
						if err != nil {
							printLog("FAILED", job.Name, fmt.Sprintf("Backup polling failed: %v", err), ui.ErrorColor)
							job.Status = "failed"
							job.Error = err
							backupSuccess = false
							break pollLoop
						}
					}
				}

				if !backupSuccess {
					<-sem
					continue
				}

				// Execute SSH WP-CLI
				printLog("UPDATE", job.Name, "Running updates via WP-CLI...", ui.PrimaryColor)

				wpArgs := []string{"plugin", "update"}
				if updateDryRun {
					wpArgs = append(wpArgs, "--dry-run")
				}

				var stdout, stderr string
				if scope == "all" {
					scopes := []string{"plugin", "theme", "core"}
					for _, s := range scopes {
						args := []string{s, "update"}
						if updateDryRun {
							args = append(args, "--dry-run")
						}
						if s != "core" {
							args = append(args, "--all")
						}
						stdout, stderr, err = SSHClient.RunWPCLI(job.Name, args...)
						if err != nil {
							break
						}
					}
				} else {
					if scope == "themes" {
						wpArgs[0] = "theme"
						wpArgs = append(wpArgs, "--all")
					} else if scope == "core" {
						wpArgs[0] = "core"
					} else {
						wpArgs = append(wpArgs, "--all")
					}
					stdout, stderr, err = SSHClient.RunWPCLI(job.Name, wpArgs...)
				}

				if err != nil {
					printLog("FAILED", job.Name, fmt.Sprintf("Update failed: %v (stderr: %s)", err, stderr), ui.ErrorColor)
					job.Status = "failed"
					job.Error = err
				} else {
					summary := strings.TrimSpace(stdout)
					if summary == "" {
						summary = "No updates required or completed successfully."
					} else {
						lines := strings.Split(summary, "\n")
						summary = lines[len(lines)-1] // Show last line for brevity
					}
					printLog("SUCCESS", job.Name, summary, ui.SuccessColor)
					job.Status = "completed"
					job.Details = summary
				}

				<-sem
			}
		}()
	}

	wg.Wait()
	if OutputFormat != "json" {
		fmt.Println()
		renderUpdateSummaryTable(jobs)
		fmt.Println(lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render(ux.Symbol("check", PlainOutput)+" All operations completed.") + "\n")
	}
}

func renderUpdateSummaryTable(jobs []*ui.Job) {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ui.PrimaryColor)).
		Headers("Environment", "Type", "Status", "Details")

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

	for _, job := range jobs {
		statusStr := lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("SUCCESS")
		if job.Status == "failed" {
			statusStr = lipgloss.NewStyle().Foreground(ui.ErrorColor).Bold(true).Render("FAILED")
		}

		details := job.Details
		if job.Error != nil {
			details = job.Error.Error()
		}
		if len(details) > 60 {
			details = details[:57] + "..."
		}

		envType := shortEnvType(job.EnvType)
		var typeStr string
		switch envType {
		case "prod":
			typeStr = lipgloss.NewStyle().Foreground(ui.ErrorColor).Bold(true).Render("PROD")
		case "stg":
			typeStr = lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true).Render("STG")
		default:
			typeStr = lipgloss.NewStyle().Foreground(ui.InfoColor).Bold(true).Render("DEV")
		}

		t.Row(job.Name, typeStr, statusStr, details)
	}

	fmt.Println(t.Render())
}

func init() {
	updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update WordPress core, plugins, and themes")
	updateCmd.Flags().BoolVar(&updatePlugins, "plugins", false, "Update plugins only (default)")
	updateCmd.Flags().BoolVar(&updateThemes, "themes", false, "Update themes only")
	updateCmd.Flags().BoolVar(&updateCore, "core", false, "Update WordPress core only")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Run updates with --dry-run")
	updateCmd.Flags().StringVar(&updateBatch, "batch", "", "Comma-separated list of environment names, or path to a text file with targets")
	updateCmd.Flags().BoolVar(&updateAllEnvs, "all-envs", false, "Update all active environments under the account")
	updateCmd.Flags().IntVar(&updateConcurrent, "concurrency", 0, "Concurrency limit for updates (falls back to config batch_concurrency)")
	updateCmd.Flags().StringVarP(&updateEmail, "email", "e", "", "Email address for WP Engine backup completion notifications")

	RootCmd.AddCommand(updateCmd)
}

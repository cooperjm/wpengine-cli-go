package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
				ID:     cachedInst.ID,
				Name:   cachedInst.Name,
				Status: "idle",
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
					ID:     inst.ID,
					Name:   inst.Name,
					Status: "idle",
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

	// Run Execution Loop
	// Select interactive TUI or static log mode
	interactive := Cfg.Interactive && !NoInter
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

	return nil
}

// runNonInteractive executes the updates using clean Lipgloss-styled print statements (useful for scripting/CI).
func runNonInteractive(jobs []*ui.Job, scope string, concurrency int) {
	fmt.Println("\n" + ui.GetStatusBadge("updating") + " Starting Batch Updates (Non-Interactive Mode)")
	fmt.Printf("Targets: %d | Concurrency: %d | Scope: %s\n\n", len(jobs), concurrency, scope)

	jobChan := make(chan int, len(jobs))
	for i := range jobs {
		jobChan <- i
	}
	close(jobChan)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	var logMu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobChan {
				sem <- struct{}{}

				job := jobs[idx]
				if job.Status == "failed" {
					logMu.Lock()
					ui.PrintLog("FAILED", job.Name, job.Error.Error(), lipgloss.Color("196"))
					logMu.Unlock()
					<-sem
					continue
				}

				// SSH Verify
				logMu.Lock()
				ui.PrintLog("VERIFY", job.Name, "Testing SSH gateway connection...", lipgloss.Color("39"))
				logMu.Unlock()
				err := SSHClient.VerifyConnection(job.Name)
				if err != nil {
					logMu.Lock()
					ui.PrintLog("FAILED", job.Name, fmt.Sprintf("SSH connection failed: %v", err), lipgloss.Color("196"))
					logMu.Unlock()
					job.Status = "failed"
					job.Error = err
					<-sem
					continue
				}

				// Trigger Backup
				logMu.Lock()
				ui.PrintLog("BACKUP", job.Name, "Creating backup checkpoint...", lipgloss.Color("214"))
				logMu.Unlock()

				backupDesc := fmt.Sprintf("cli-pre-update-%d", time.Now().Unix())
				var emails []string
				if updateEmail != "" {
					emails = []string{updateEmail}
				}
				backup, err := APIClient.CreateBackup(job.ID, backupDesc, emails)
				if err != nil {
					logMu.Lock()
					ui.PrintLog("FAILED", job.Name, fmt.Sprintf("Backup failed: %v", err), lipgloss.Color("196"))
					logMu.Unlock()
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
						logMu.Lock()
						ui.PrintLog("BACKUP", job.Name, fmt.Sprintf("Polling status: %s", b.Status), lipgloss.Color("214"))
						logMu.Unlock()
					case err := <-errChan:
						if err != nil {
							logMu.Lock()
							ui.PrintLog("FAILED", job.Name, fmt.Sprintf("Backup polling failed: %v", err), lipgloss.Color("196"))
							logMu.Unlock()
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
				logMu.Lock()
				ui.PrintLog("UPDATE", job.Name, "Running updates via WP-CLI...", lipgloss.Color("99"))
				logMu.Unlock()

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

				logMu.Lock()
				if err != nil {
					ui.PrintLog("FAILED", job.Name, fmt.Sprintf("Update failed: %v (stderr: %s)", err, stderr), lipgloss.Color("196"))
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
					ui.PrintLog("SUCCESS", job.Name, summary, lipgloss.Color("46"))
					job.Status = "completed"
				}
				logMu.Unlock()

				<-sem
			}
		}()
	}

	wg.Wait()
	fmt.Println("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render("✔ All operations completed.") + "\n")
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

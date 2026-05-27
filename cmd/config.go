package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	setUsername      string
	setPassword      string
	setAccountID     string
	setSSHKeyPath    string
	setSSHPassphrase string
	setBatchLimit    int
	setInteractive   string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure WP Engine CLI settings",
	Long:  `Manage your WP Engine API credentials, SSH keys, and general preferences.`,
	Example: `  wpengine config configure
  wpengine config set --username <api_user> --password <api_pass> --account-id <account_uuid>
  wpengine config show`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		banner := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Render("WP Engine CLI Configuration")
		fmt.Println("\n" + banner)
		fmt.Println(strings.Repeat("-", 40))

		// Mask password for security
		maskedPassword := "[NOT SET]"
		if Cfg.Password != "" {
			maskedPassword = strings.Repeat("*", 8)
		}

		bold := lipgloss.NewStyle().Bold(true)
		fmt.Printf("%-20s %s\n", bold.Render("API Username:"), Cfg.Username)
		fmt.Printf("%-20s %s\n", bold.Render("API Password:"), maskedPassword)
		fmt.Printf("%-20s %s\n", bold.Render("Default Account ID:"), Cfg.AccountID)
		fmt.Printf("%-20s %s\n", bold.Render("SSH Key Path:"), Cfg.SSHKeyPath)

		maskedPassphrase := "[NOT SET]"
		if Cfg.SSHKeyPassphrase != "" {
			maskedPassphrase = strings.Repeat("*", 8)
		}
		fmt.Printf("%-20s %s\n", bold.Render("SSH Passphrase:"), maskedPassphrase)
		fmt.Printf("%-20s %d\n", bold.Render("Batch Concurrency:"), Cfg.BatchConcurrency)
		fmt.Printf("%-20s %t\n", bold.Render("TUI Interactivity:"), Cfg.Interactive)
		fmt.Println(strings.Repeat("-", 40))

		path, err := config.GetConfigPath()
		if err != nil {
			fmt.Printf("Config file location: unavailable (%v)\n\n", err)
		} else {
			fmt.Printf("Config file location: %s\n\n", path)
		}
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set configuration fields",
	RunE: func(cmd *cobra.Command, args []string) error {
		updated := false

		if setUsername != "" {
			Cfg.Username = setUsername
			updated = true
		}
		if setPassword != "" {
			Cfg.Password = setPassword
			updated = true
		}
		if setAccountID != "" {
			Cfg.AccountID = setAccountID
			updated = true
		}
		if setSSHKeyPath != "" {
			Cfg.SSHKeyPath = setSSHKeyPath
			updated = true
		}
		if setSSHPassphrase != "" {
			Cfg.SSHKeyPassphrase = setSSHPassphrase
			updated = true
		}
		if cmd.Flags().Changed("batch-concurrency") {
			Cfg.BatchConcurrency = setBatchLimit
			updated = true
		}
		if setInteractive != "" {
			if setInteractive == "true" || setInteractive == "yes" {
				Cfg.Interactive = true
			} else {
				Cfg.Interactive = false
			}
			updated = true
		}

		if !updated {
			return fmt.Errorf("no flags provided to set. Use: wpengine config set [flags]")
		}

		if err := Cfg.Save(); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		badge := lipgloss.NewStyle().Background(lipgloss.Color("46")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" SUCCESS ")
		fmt.Printf("\n%s Configuration updated and saved successfully!\n\n", badge)
		return nil
	},
}

var configConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Run an interactive first-time setup flow",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println()
		fmt.Println(PrimaryStyle.Render("WP Engine CLI Setup"))
		fmt.Println("Press Enter to keep an existing value.")
		fmt.Println()

		username, err := promptString(reader, "API username", Cfg.Username)
		if err != nil {
			return err
		}
		if username != "" {
			Cfg.Username = username
		}

		password, err := promptSecret("API password", Cfg.Password != "")
		if err != nil {
			return err
		}
		if password != "" {
			Cfg.Password = password
		}

		if Cfg.Username != "" && Cfg.Password != "" {
			APIClient = nil
			if Cfg.Username != "" && Cfg.Password != "" {
				APIClient = api.NewClient(Cfg.Username, Cfg.Password)
			}
			if APIClient != nil {
				accounts, err := APIClient.GetAccounts(100, 0)
				if err != nil {
					fmt.Printf("Warning: API credentials were saved, but account lookup failed: %v\n", err)
				} else if len(accounts.Results) > 0 {
					fmt.Println()
					fmt.Println("Available accounts:")
					for i, acc := range accounts.Results {
						name := acc.Name
						if acc.Nickname != "" {
							name = fmt.Sprintf("%s (%s)", acc.Name, acc.Nickname)
						}
						fmt.Printf("  %d. %s - %s\n", i+1, name, acc.ID)
					}
					defaultAccount := Cfg.AccountID
					if defaultAccount == "" {
						defaultAccount = accounts.Results[0].ID
					}
					accountID, err := promptString(reader, "Default account ID", defaultAccount)
					if err != nil {
						return err
					}
					if accountID != "" {
						Cfg.AccountID = accountID
					}
				}
			}
		}

		sshDefault := Cfg.SSHKeyPath
		if sshDefault == "" {
			sshDefault = detectSSHKeyPath()
		}
		sshPath, err := promptString(reader, "SSH private key path", sshDefault)
		if err != nil {
			return err
		}
		if sshPath != "" {
			Cfg.SSHKeyPath = expandHome(sshPath)
		}

		passphrase, err := promptSecret("SSH key passphrase", Cfg.SSHKeyPassphrase != "")
		if err != nil {
			return err
		}
		if passphrase != "" {
			Cfg.SSHKeyPassphrase = passphrase
		}

		if err := Cfg.Save(); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		badge := lipgloss.NewStyle().Background(lipgloss.Color("46")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" SUCCESS ")
		fmt.Printf("\n%s Setup saved. Run `wpengine doctor` to verify API, SSH, cache, and terminal readiness.\n\n", badge)
		return nil
	},
}

func init() {
	configSetCmd.Flags().StringVar(&setUsername, "username", "", "WP Engine API Username")
	configSetCmd.Flags().StringVar(&setPassword, "password", "", "WP Engine API Password")
	configSetCmd.Flags().StringVar(&setAccountID, "account-id", "", "Default WP Engine Account ID")
	configSetCmd.Flags().StringVar(&setSSHKeyPath, "ssh-key-path", "", "Path to SSH private key file")
	configSetCmd.Flags().StringVar(&setSSHPassphrase, "ssh-passphrase", "", "Passphrase for decrypting SSH private key")
	configSetCmd.Flags().IntVar(&setBatchLimit, "batch-concurrency", 10, "Maximum number of parallel updates")
	configSetCmd.Flags().StringVar(&setInteractive, "interactive", "", "Enable or disable interactive TUI dashboard (true/false)")

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configConfigureCmd)
	RootCmd.AddCommand(configCmd)
}

func promptString(reader *bufio.Reader, label string, current string) (string, error) {
	if current != "" {
		fmt.Printf("%s [%s]: ", label, current)
	} else {
		fmt.Printf("%s: ", label)
	}
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return current, nil
	}
	return value, nil
}

func promptSecret(label string, hasCurrent bool) (string, error) {
	if hasCurrent {
		fmt.Printf("%s [configured, press Enter to keep]: ", label)
	} else {
		fmt.Printf("%s: ", label)
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(value)), nil
	}
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func detectSSHKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

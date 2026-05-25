package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	setUsername       string
	setPassword       string
	setAccountID      string
	setSSHKeyPath     string
	setSSHPassphrase  string
	setBatchLimit     int
	setInteractive    string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure WP Engine CLI settings",
	Long:  `Manage your WP Engine API credentials, SSH keys, and general preferences.`,
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
		
		path, _ := cmd.Flags().GetString("config")
		if path == "" {
			importPath, _ := cmd.Flags().GetString("config")
			if importPath == "" {
				// Get standard default path
				importPath = "~/.wpengine-cli.yaml"
			}
			fmt.Printf("Config file location: %s\n\n", importPath)
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

func init() {
	configSetCmd.Flags().StringVar(&setUsername, "username", "", "WP Engine API Username")
	configSetCmd.Flags().StringVar(&setPassword, "password", "", "WP Engine API Password")
	configSetCmd.Flags().StringVar(&setAccountID, "account-id", "", "Default WP Engine Account ID")
	configSetCmd.Flags().StringVar(&setSSHKeyPath, "ssh-key-path", "", "Path to SSH private key file")
	configSetCmd.Flags().StringVar(&setSSHPassphrase, "ssh-passphrase", "", "Passphrase for decrypting SSH private key")
	configSetCmd.Flags().IntVar(&setBatchLimit, "batch-concurrency", 3, "Maximum number of parallel updates")
	configSetCmd.Flags().StringVar(&setInteractive, "interactive", "", "Enable or disable interactive TUI dashboard (true/false)")

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	RootCmd.AddCommand(configCmd)
}

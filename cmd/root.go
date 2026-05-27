package cmd

import (
	"fmt"
	"os"
	"strings"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ssh"
	"wpengine-cli/internal/ui"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	CfgFile      string
	Verbose      bool
	NoInter      bool
	PlainOutput  bool
	OutputFormat string
	AssumeYes    bool
	Cfg          *config.Config
	APIClient    *api.Client
	SSHClient    *ssh.Client
	PrimaryStyle lipgloss.Style
)

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:           "wpengine",
	Short:         "WP Engine Management CLI",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `A premium Go CLI tool to manage your WP Engine sites, environments (installs), 
backups, and automated updates with secure SSH execution.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if OutputFormat != "" {
			OutputFormat = strings.ToLower(OutputFormat)
		}
		if OutputFormat == "" {
			OutputFormat = "text"
		}
		if OutputFormat != "text" && OutputFormat != "json" {
			return fmt.Errorf("unsupported output format %q (use text or json)", OutputFormat)
		}
		if CfgFile != "" {
			config.SetConfigPath(CfgFile)
		}
		ui.SetPlainOutput(PlainOutput)

		// Load config
		var err error
		Cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Instantiate clients
		if Cfg.Username != "" && Cfg.Password != "" {
			APIClient = api.NewClient(Cfg.Username, Cfg.Password)
		}
		SSHClient = ssh.NewClient(Cfg.SSHKeyPath, Cfg.SSHKeyPassphrase)

		// Setup UI styles
		PrimaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVar(&CfgFile, "config", "", "config file (default is $HOME/.wpengine-cli.yaml)")
	RootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "enable verbose output")
	RootCmd.PersistentFlags().BoolVar(&NoInter, "no-interactive", false, "disable interactive TUI dashboard (useful for CI/CD)")
	RootCmd.PersistentFlags().BoolVar(&PlainOutput, "plain", false, "disable colors, borders, and Unicode symbols")
	RootCmd.PersistentFlags().StringVar(&OutputFormat, "output", "text", "output format: text or json")
	RootCmd.PersistentFlags().BoolVarP(&AssumeYes, "yes", "y", false, "skip confirmation prompts")
}

// RequireAPI ensures the API client is fully configured before running API-dependent commands.
func RequireAPI() error {
	if APIClient == nil {
		badge := lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" ERROR ")
		fmt.Printf("\n%s WP Engine API credentials are not configured.\n", badge)
		fmt.Println("Please run: wpengine config set --username <api_user> --password <api_pass>")
		return fmt.Errorf("API credentials not configured")
	}
	return nil
}

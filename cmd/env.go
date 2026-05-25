package cmd

import (
	"fmt"

	"wpengine-cli/internal/api"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
)

var (
	envAccountID   string
	envSiteID      string
	envType        string
	envName        string
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage WP Engine environments (installs)",
}

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments (installs) for the configured account",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return RequireAPI()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		installsResp, err := APIClient.GetInstalls(100, 0)
		if err != nil {
			return fmt.Errorf("failed to fetch environments: %w", err)
		}

		if len(installsResp.Results) == 0 {
			fmt.Println("\nNo environments found.")
			return nil
		}

		fmt.Println("\n" + PrimaryStyle.Render("WP Engine Environments (Installs)") + "\n")

		// Create a Lipgloss table
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
			Headers("ID", "Name", "Environment", "CNAME", "Primary Domain", "Status")

		// Styling table headers
		t.StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color("255")).
					Background(lipgloss.Color("99")).
					Bold(true).
					Padding(0, 1)
			}

			// Safety check: ensure row is within bounds of results
			if row < 0 || row >= len(installsResp.Results) {
				return lipgloss.NewStyle().Padding(0, 1)
			}
			inst := installsResp.Results[row]

			// Colorize env column
			if col == 2 {
				switch inst.Environment {
				case "production":
					return lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Bold(true).Padding(0, 1) // red
				case "staging":
					return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Padding(0, 1) // amber
				default:
					return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Padding(0, 1)  // blue
				}
			}
			// Colorize status column
			if col == 5 {
				if inst.Status == "active" {
					return lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Padding(0, 1) // green
				}
				return lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Padding(0, 1) // gray
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})

		for _, inst := range installsResp.Results {
			t.Row(inst.ID, inst.Name, inst.Environment, inst.CNAME, inst.PrimaryDomain, inst.Status)
		}

		fmt.Println(t.Render())
		fmt.Printf("\nTotal environments found: %d\n\n", len(installsResp.Results))
		return nil
	},
}

var envCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create/Spin up a new environment (install)",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return RequireAPI()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if envName == "" {
			return fmt.Errorf("environment name (--name) is required")
		}

		accID := envAccountID
		if accID == "" {
			accID = Cfg.AccountID
		}
		if accID == "" {
			// Auto-fetch the first account if default account ID is not set
			accounts, err := APIClient.GetAccounts(1, 0)
			if err != nil || len(accounts.Results) == 0 {
				return fmt.Errorf("account ID (--account-id) is required and none was configured or could be retrieved")
			}
			accID = accounts.Results[0].ID
		}

		req := &api.CreateInstallRequest{
			Name:        envName,
			AccountID:   accID,
			SiteID:      envSiteID,
			Environment: envType,
		}

		badgeProgress := lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(" CREATING ")
		fmt.Printf("\n%s Requesting environment creation for: %s...\n", badgeProgress, envName)

		install, err := APIClient.CreateInstall(req)
		if err != nil {
			return fmt.Errorf("failed to create environment: %w", err)
		}

		badgeSuccess := lipgloss.NewStyle().Background(lipgloss.Color("46")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" SUCCESS ")
		fmt.Printf("\n%s Environment '%s' created successfully!\n", badgeSuccess, install.Name)
		fmt.Printf("ID:    %s\n", install.ID)
		fmt.Printf("CNAME: %s\n\n", install.CNAME)

		return nil
	},
}

var envDeleteCmd = &cobra.Command{
	Use:   "delete <install_id>",
	Short: "Delete an environment (install)",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return RequireAPI()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		installID := args[0]

		badgeWarning := lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" DELETING ")
		fmt.Printf("\n%s Requesting deletion of environment ID: %s...\n", badgeWarning, installID)

		err := APIClient.DeleteInstall(installID)
		if err != nil {
			return fmt.Errorf("failed to delete environment: %w", err)
		}

		badgeSuccess := lipgloss.NewStyle().Background(lipgloss.Color("46")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" SUCCESS ")
		fmt.Printf("\n%s Environment deleted successfully.\n\n", badgeSuccess)
		return nil
	},
}

func init() {
	envCreateCmd.Flags().StringVar(&envName, "name", "", "Name of the new environment (required)")
	envCreateCmd.Flags().StringVar(&envAccountID, "account-id", "", "Account ID (falls back to default account in config)")
	envCreateCmd.Flags().StringVar(&envSiteID, "site-id", "", "Site ID this environment belongs to (optional)")
	envCreateCmd.Flags().StringVar(&envType, "type", "development", "Environment type: production, staging, development")

	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envCreateCmd)
	envCmd.AddCommand(envDeleteCmd)
	RootCmd.AddCommand(envCmd)
}

package cmd

import (
	"fmt"
	"os"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ux"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	envAccountID string
	envSiteID    string
	envType      string
	envName      string

	envListProd bool
	envListStg  bool
	envListDev  bool
	envListNames bool
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
		installs, err := APIClient.GetAllInstalls()
		if err != nil {
			return fmt.Errorf("failed to fetch environments: %w", err)
		}

		if len(installs) == 0 {
			fmt.Println("\nNo environments found.")
			return nil
		}

		// Save the retrieved installs to the local cache
		if err := config.SaveCache(installs); err != nil {
			fmt.Printf("Warning: failed to save environments cache: %v\n", err)
		}

		filtering := envListProd || envListStg || envListDev

		var filteredInstalls []api.Install
		for _, inst := range installs {
			if filtering {
				if envListProd && inst.Environment == "production" {
					filteredInstalls = append(filteredInstalls, inst)
					continue
				}
				if envListStg && inst.Environment == "staging" {
					filteredInstalls = append(filteredInstalls, inst)
					continue
				}
				if envListDev && inst.Environment == "development" {
					filteredInstalls = append(filteredInstalls, inst)
					continue
				}
				continue
			}
			filteredInstalls = append(filteredInstalls, inst)
		}

		if len(filteredInstalls) == 0 {
			if envListNames {
				return nil
			}
			fmt.Println("\nNo environments matching the filters were found.")
			return nil
		}

		if envListNames {
			for _, inst := range filteredInstalls {
				fmt.Println(inst.Name)
			}
			return nil
		}

		fmt.Println("\n" + PrimaryStyle.Render("WP Engine Environments (Installs)") + "\n")

		// Detect terminal width
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width <= 0 {
			width = 120
		}
		if width > 140 {
			width = 140
		}

		// Create a Lipgloss table
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
			Headers("ID", "Name", "Environment", "CNAME", "Primary Domain", "Status").
			Width(width).
			Wrap(true)

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
			if row < 0 || row >= len(filteredInstalls) {
				return lipgloss.NewStyle().Padding(0, 1)
			}
			inst := filteredInstalls[row]

			// Colorize env column
			if col == 2 {
				switch inst.Environment {
				case "production":
					return lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Bold(true).Padding(0, 1) // red
				case "staging":
					return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Padding(0, 1) // amber
				default:
					return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Padding(0, 1) // blue
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

		for _, inst := range filteredInstalls {
			t.Row(inst.ID, inst.Name, inst.Environment, inst.CNAME, inst.PrimaryDomain, inst.Status)
		}

		fmt.Println(t.Render())
		fmt.Printf("\nTotal environments found: %d\n\n", len(filteredInstalls))
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

		ok, err := ux.Confirm(fmt.Sprintf("Delete environment %s? This cannot be undone.", installID), AssumeYes)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("delete cancelled")
		}

		badgeWarning := lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" DELETING ")
		fmt.Printf("\n%s Requesting deletion of environment ID: %s...\n", badgeWarning, installID)

		if err := APIClient.DeleteInstall(installID); err != nil {
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

	envListCmd.Flags().BoolVarP(&envListProd, "production", "p", false, "Filter to only production environments")
	envListCmd.Flags().BoolVarP(&envListStg, "staging", "s", false, "Filter to only staging environments")
	envListCmd.Flags().BoolVarP(&envListDev, "dev", "d", false, "Filter to only development environments")
	envListCmd.Flags().BoolVar(&envListNames, "names", false, "Print only environment names (one per line) for batch files")

	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envCreateCmd)
	envCmd.AddCommand(envDeleteCmd)
	RootCmd.AddCommand(envCmd)
}

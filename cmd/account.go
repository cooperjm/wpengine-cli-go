package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage or list WP Engine accounts",
}

var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all WP Engine accounts you have access to",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return RequireAPI()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		accountsResp, err := APIClient.GetAccounts(100, 0)
		if err != nil {
			return fmt.Errorf("failed to fetch accounts: %w", err)
		}

		if len(accountsResp.Results) == 0 {
			fmt.Println("\nNo accounts found associated with these API credentials.")
			return nil
		}

		fmt.Println("\n" + PrimaryStyle.Render("WP Engine Accounts") + "\n")

		// Create a Lipgloss table
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
			Headers("Account ID (UUID)", "Name", "Nickname")

		t.StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color("255")).
					Background(lipgloss.Color("99")).
					Bold(true).
					Padding(0, 1)
			}
			// Bold the UUID column for easy copying
			if col == 0 {
				return lipgloss.NewStyle().Bold(true).Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})

		for _, acc := range accountsResp.Results {
			t.Row(acc.ID, acc.Name, acc.Nickname)
		}

		fmt.Println(t.Render())
		fmt.Printf("\nTotal accounts found: %d\n\n", len(accountsResp.Results))
		return nil
	},
}

func init() {
	accountCmd.AddCommand(accountListCmd)
	RootCmd.AddCommand(accountCmd)
}

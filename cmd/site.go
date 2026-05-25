package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Manage WP Engine sites",
}

var siteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sites for the configured account",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return RequireAPI()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		sitesResp, err := APIClient.GetSites(100, 0)
		if err != nil {
			return fmt.Errorf("failed to fetch sites: %w", err)
		}

		if len(sitesResp.Results) == 0 {
			fmt.Println("\nNo sites found.")
			return nil
		}

		fmt.Println("\n" + PrimaryStyle.Render("WP Engine Sites") + "\n")

		// Create a Lipgloss table
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
			Headers("ID", "Name", "Associated Account ID")

		t.StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color("255")).
					Background(lipgloss.Color("99")).
					Bold(true).
					Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})

		for _, site := range sitesResp.Results {
			t.Row(site.ID, site.Name, site.Account.ID)
		}

		fmt.Println(t.Render())
		fmt.Printf("\nTotal sites found: %d\n\n", len(sitesResp.Results))
		return nil
	},
}

func init() {
	siteCmd.AddCommand(siteListCmd)
	RootCmd.AddCommand(siteCmd)
}

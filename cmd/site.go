package cmd

import (
	"fmt"
	"strings"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
)

var (
	siteListProd bool
	siteListStg  bool
	siteListDev  bool

	prodStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Bold(true)
	stgStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	devStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
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
		sites, err := APIClient.GetAllSites()
		if err != nil {
			return fmt.Errorf("failed to fetch sites: %w", err)
		}

		if len(sites) == 0 {
			fmt.Println("\nNo sites found.")
			return nil
		}

		// Fetch installs (environments) to associate with sites
		installs, err := APIClient.GetAllInstalls()
		if err != nil {
			return fmt.Errorf("failed to fetch environments: %w", err)
		}

		// Save the retrieved installs to the local cache
		if err := config.SaveCache(installs); err != nil {
			fmt.Printf("Warning: failed to save environments cache: %v\n", err)
		}

		// Group installs by Site ID
		siteInstalls := make(map[string][]api.Install)
		for _, inst := range installs {
			if inst.Site.ID != "" {
				siteInstalls[inst.Site.ID] = append(siteInstalls[inst.Site.ID], inst)
			}
		}

		filtering := siteListProd || siteListStg || siteListDev

		// Filter sites based on environment type flags
		var filteredSites []api.Site
		for _, site := range sites {
			insts := siteInstalls[site.ID]

			if filtering {
				match := false
				for _, inst := range insts {
					if siteListProd && inst.Environment == "production" {
						match = true
						break
					}
					if siteListStg && inst.Environment == "staging" {
						match = true
						break
					}
					if siteListDev && inst.Environment == "development" {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}
			filteredSites = append(filteredSites, site)
		}

		if len(filteredSites) == 0 {
			fmt.Println("\nNo sites matching the environment filters were found.")
			return nil
		}

		fmt.Println("\n" + PrimaryStyle.Render("WP Engine Sites") + "\n")

		// Create a Lipgloss table
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("99"))).
			Headers("ID", "Name", "Environments", "Associated Account ID")

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

		for _, site := range filteredSites {
			insts := siteInstalls[site.ID]

			// Format environments string
			var envStrings []string
			for _, inst := range insts {
				var formatted string
				switch inst.Environment {
				case "production":
					formatted = prodStyle.Render("prod") + fmt.Sprintf(" (%s)", inst.Name)
				case "staging":
					formatted = stgStyle.Render("stage") + fmt.Sprintf(" (%s)", inst.Name)
				case "development":
					formatted = devStyle.Render("dev") + fmt.Sprintf(" (%s)", inst.Name)
				default:
					formatted = inst.Environment + fmt.Sprintf(" (%s)", inst.Name)
				}
				envStrings = append(envStrings, formatted)
			}

			envListStr := "-"
			if len(envStrings) > 0 {
				envListStr = strings.Join(envStrings, ", ")
			}

			t.Row(site.ID, site.Name, envListStr, site.Account.ID)
		}

		fmt.Println(t.Render())
		fmt.Printf("\nTotal sites found: %d\n\n", len(filteredSites))
		return nil
	},
}

func init() {
	siteListCmd.Flags().BoolVarP(&siteListProd, "production", "p", false, "Filter sites to only those with a production environment")
	siteListCmd.Flags().BoolVarP(&siteListStg, "staging", "s", false, "Filter sites to only those with a staging environment")
	siteListCmd.Flags().BoolVarP(&siteListDev, "dev", "d", false, "Filter sites to only those with a development environment")

	siteCmd.AddCommand(siteListCmd)
	RootCmd.AddCommand(siteCmd)
}

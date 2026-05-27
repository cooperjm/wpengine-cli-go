package cmd

import (
	"fmt"
	"strings"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ui"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	siteListProd bool
	siteListStg  bool
	siteListDev  bool
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
		var sites []api.Site
		if err := ui.RunWithSpinner("Fetching sites...", PlainOutput, func() error {
			var e error
			sites, e = APIClient.GetAllSites()
			return e
		}); err != nil {
			return fmt.Errorf("failed to fetch sites: %w", err)
		}

		if len(sites) == 0 {
			fmt.Println("\nNo sites found.")
			return nil
		}

		var installs []api.Install
		if err := ui.RunWithSpinner("Fetching environments...", PlainOutput, func() error {
			var e error
			installs, e = APIClient.GetAllInstalls()
			return e
		}); err != nil {
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

		renderSiteTree(filteredSites, siteInstalls, PrimaryStyle)
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

func renderSiteTree(sites []api.Site, siteInstalls map[string][]api.Install, primaryStyle lipgloss.Style) {
	siteHeaderStyle := lipgloss.NewStyle().Foreground(ui.PrimaryColor).Bold(true)
	siteIDStyle := lipgloss.NewStyle().Foreground(ui.MutedColor)
	domainStyle := lipgloss.NewStyle().Foreground(ui.MutedColor)
	activeStyle := lipgloss.NewStyle().Foreground(ui.SuccessColor)
	inactiveStyle := lipgloss.NewStyle().Foreground(ui.MutedColor)
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(strings.Repeat("─", 60))

	totalEnvs := 0
	for _, site := range sites {
		totalEnvs += len(siteInstalls[site.ID])
	}

	fmt.Println()
	fmt.Println(primaryStyle.Render("WP Engine Sites"))
	fmt.Println()

	for i, site := range sites {
		if i > 0 {
			fmt.Println(divider)
		}
		fmt.Printf("%s %s  %s\n",
			siteHeaderStyle.Render("⬡"),
			siteHeaderStyle.Render(site.Name),
			siteIDStyle.Render(site.ID),
		)

		for _, inst := range siteInstalls[site.ID] {
			badge := siteEnvBadge(inst.Environment)
			statusStr := activeStyle.Render("● active")
			if inst.Status != "active" {
				statusStr = inactiveStyle.Render("○ " + inst.Status)
			}
			domain := inst.PrimaryDomain
			if domain == "" {
				domain = inst.CNAME
			}
			fmt.Printf("   %s  %-30s  %-40s  %s\n",
				badge,
				inst.Name,
				domainStyle.Render(domain),
				statusStr,
			)
		}
	}

	fmt.Printf("\n%s\n\n",
		lipgloss.NewStyle().Foreground(ui.MutedColor).Render(
			fmt.Sprintf("%d sites · %d environments", len(sites), totalEnvs),
		),
	)
}

func siteEnvBadge(environment string) string {
	switch environment {
	case "production":
		return lipgloss.NewStyle().Background(lipgloss.Color("88")).Foreground(lipgloss.Color("217")).Bold(true).Padding(0, 1).Render("PROD")
	case "staging":
		return lipgloss.NewStyle().Background(lipgloss.Color("94")).Foreground(lipgloss.Color("229")).Bold(true).Padding(0, 1).Render(" STG")
	case "development":
		return lipgloss.NewStyle().Background(lipgloss.Color("17")).Foreground(lipgloss.Color("117")).Bold(true).Padding(0, 1).Render(" DEV")
	default:
		label := environment
		if len(label) > 3 {
			label = label[:3]
		}
		return lipgloss.NewStyle().Background(ui.MutedColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(strings.ToUpper(label))
	}
}

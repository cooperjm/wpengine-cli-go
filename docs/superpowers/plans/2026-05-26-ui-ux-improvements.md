# UI/UX Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve CLI readability and usability across all four core commands by adding loading spinners, redesigning the site list as a grouped tree, flipping check output defaults, adding elapsed time to the update dashboard, and adding summary lines to doctor and non-interactive update output.

**Architecture:** Extract a shared style palette into `internal/ui/styles.go` and a reusable spinner into `internal/ui/spinner.go`. Remaining changes are isolated to their respective command files (`cmd/site.go`, `cmd/env.go`, `cmd/check.go`, `cmd/update.go`, `cmd/doctor.go`) and `internal/ui/dashboard.go`.

**Tech Stack:** Go, Cobra (CLI), Bubble Tea (TUI), Lipgloss (styling), `golang.org/x/term` (terminal detection)

---

## File Map

| Action | File | Purpose |
|---|---|---|
| Create | `internal/ui/styles.go` | Exported color palette and shared style helpers |
| Create | `internal/ui/spinner.go` | `RunWithSpinner` — wraps an API call with an animated spinner |
| Create | `internal/ui/spinner_test.go` | Tests for `RunWithSpinner` |
| Modify | `internal/ui/dashboard.go` | Use exported colors; add elapsed time per job and total |
| Modify | `cmd/check.go` | Summary table as default; add `--verbose`; deprecate `--minimal`; use `ui.WarningColor` etc. |
| Modify | `cmd/check_test.go` | Add tests for default vs `--verbose` output |
| Modify | `cmd/site.go` | Add spinner; replace table with grouped tree renderer |
| Modify | `cmd/env.go` | Add spinner; sort installs by `Site.ID` |
| Modify | `cmd/update.go` | Add final results table to non-interactive mode; use exported colors |
| Modify | `cmd/doctor.go` | Add summary line; extract `doctorSummary` helper |
| Modify | `README.md` | Update `check`, `site list`, and Architecture sections |

---

## Task 1: Shared Style Package

**Files:**
- Create: `internal/ui/styles.go`
- Modify: `internal/ui/dashboard.go` (replace local color vars)

- [ ] **Step 1: Create `internal/ui/styles.go`**

```go
package ui

import "github.com/charmbracelet/lipgloss"

var (
	PrimaryColor = lipgloss.Color("99")  // indigo/purple
	SuccessColor = lipgloss.Color("46")  // emerald green
	WarningColor = lipgloss.Color("214") // amber
	ErrorColor   = lipgloss.Color("196") // red
	InfoColor    = lipgloss.Color("39")  // sky blue
	MutedColor   = lipgloss.Color("244") // slate gray
)
```

- [ ] **Step 2: Update `internal/ui/dashboard.go` — replace local color declarations**

Remove the six unexported color vars at the top of dashboard.go:
```go
// DELETE these lines:
primaryColor = lipgloss.Color("99")
successColor = lipgloss.Color("46")
warningColor = lipgloss.Color("214")
errorColor   = lipgloss.Color("196")
infoColor    = lipgloss.Color("39")
mutedColor   = lipgloss.Color("244")
```

Then replace every reference in the same file (styles, spinner, View, GetStatusBadge, PrintLog):

| Old | New |
|---|---|
| `primaryColor` | `PrimaryColor` |
| `successColor` | `SuccessColor` |
| `warningColor` | `WarningColor` |
| `errorColor` | `ErrorColor` |
| `infoColor` | `InfoColor` |
| `mutedColor` | `MutedColor` |

- [ ] **Step 3: Verify it compiles**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/styles.go internal/ui/dashboard.go
git commit -m "refactor: extract shared color palette to internal/ui/styles.go"
```

---

## Task 2: Reusable Spinner

**Files:**
- Create: `internal/ui/spinner.go`
- Create: `internal/ui/spinner_test.go`

- [ ] **Step 1: Write the failing tests first**

Create `internal/ui/spinner_test.go`:

```go
package ui

import (
	"errors"
	"testing"
)

func TestRunWithSpinnerPlainCallsFn(t *testing.T) {
	called := false
	err := RunWithSpinner("loading...", true, func() error {
		called = true
		return nil
	})
	if !called {
		t.Error("expected fn to be called in plain mode")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunWithSpinnerPassesError(t *testing.T) {
	expected := errors.New("api error")
	err := RunWithSpinner("loading...", true, func() error {
		return expected
	})
	if err != expected {
		t.Errorf("expected %v, got %v", expected, err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ui/... -run TestRunWithSpinner -v
```
Expected: `FAIL — RunWithSpinner undefined`

- [ ] **Step 3: Create `internal/ui/spinner.go`**

```go
package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// RunWithSpinner displays an animated spinner with message while fn executes.
// Falls back to a direct fn() call when plain is true or stdout is not a terminal.
func RunWithSpinner(message string, plain bool, fn func() error) error {
	if plain || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fn()
	}

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	style := lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	done := make(chan error, 1)

	go func() {
		done <- fn()
	}()

	i := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			fmt.Print("\r\033[K")
			return err
		case <-ticker.C:
			fmt.Printf("\r%s %s", style.Render(frames[i%len(frames)]), message)
			i++
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ui/... -run TestRunWithSpinner -v
```
Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/ui/spinner.go internal/ui/spinner_test.go
git commit -m "feat: add RunWithSpinner helper for API call loading feedback"
```

---

## Task 3: Update Dashboard — Elapsed Time

**Files:**
- Modify: `internal/ui/dashboard.go`

- [ ] **Step 1: Add `StartedAt` field to `Job` struct**

In `internal/ui/dashboard.go`, update the `Job` struct:

```go
type Job struct {
	ID        string
	Name      string
	EnvType   string
	Status    string
	Details   string
	Error     error
	StartedAt time.Time // zero when idle
}
```

`time` is already imported in dashboard.go.

- [ ] **Step 2: Add `startedAt` field to `Model` and set it in `Init`**

Add to the `Model` struct:
```go
type Model struct {
	// ... existing fields ...
	startedAt time.Time
}
```

In the `Init()` method, set it before the existing return:
```go
func (m *Model) Init() tea.Cmd {
	m.startedAt = time.Now()
	m.startWorkers()
	return tea.Batch(
		m.spinner.Tick,
		m.recvCmd(),
	)
}
```

- [ ] **Step 3: Set `job.StartedAt` when a job begins running**

At the top of `runJob`, after the `job.Status == "failed"` early return, add:

```go
func (m *Model) runJob(idx int) {
	job := m.Jobs[idx]

	if job.Status == "failed" {
		m.msgChan <- JobUpdateMsg{Index: idx, Status: "failed", Err: job.Error}
		return
	}

	m.mu.Lock()
	job.StartedAt = time.Now()
	m.mu.Unlock()

	// ... rest of runJob unchanged ...
```

- [ ] **Step 4: Update `View()` to show elapsed time per job and total**

In `View()`, replace the header line:
```go
// Old:
sb.WriteString(titleStyle.Render(" WP Engine Site Update Dashboard "))

// New:
elapsed := time.Since(m.startedAt)
elapsedStr := fmt.Sprintf("%dm %02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
sb.WriteString(titleStyle.Render(fmt.Sprintf(" WP Engine Update Dashboard   Elapsed: %s ", elapsedStr)))
```

In the job rendering loop, replace the job name line to append a duration:
```go
// Old:
sb.WriteString(fmt.Sprintf("%s %s %s\n", spinStr, badge, boldStyle.Render(name)))

// New:
var durationStr string
if !job.StartedAt.IsZero() {
    d := time.Since(job.StartedAt)
    durationStr = lipgloss.NewStyle().Foreground(MutedColor).Render(
        fmt.Sprintf(" (%dm %02ds)", int(d.Minutes()), int(d.Seconds())%60),
    )
}
sb.WriteString(fmt.Sprintf("%s %s %s%s\n", spinStr, badge, boldStyle.Render(name), durationStr))
```

Note: `job.StartedAt` must be read inside the existing `m.mu.Lock()` / `m.mu.Unlock()` block already wrapping the other job field reads. Update that block to also capture `startedAt`:

```go
m.mu.Lock()
status := job.Status
details := job.Details
err := job.Error
name := job.Name
startedAt := job.StartedAt
m.mu.Unlock()
```

Then replace `job.StartedAt` with the local `startedAt` in the duration calculation.

- [ ] **Step 5: Build to verify no compile errors**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/dashboard.go
git commit -m "feat: add elapsed time display to update dashboard"
```

---

## Task 4: `check` — Summary Table as Default

**Files:**
- Modify: `cmd/check.go`
- Modify: `cmd/check_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `cmd/check_test.go`:

```go
func TestRunChecksDefaultIsSummaryTable(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	results := []SiteCheckResult{
		{EnvName: "env-one", EnvType: "prod", Err: nil},
	}
	renderSummaryTable(results)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Environment") {
		t.Errorf("expected summary table header in output, got: %s", out)
	}
	if strings.Contains(out, "Update Report for:") {
		t.Errorf("expected no verbose box in default output, got: %s", out)
	}
}

func TestRenderCheckResultVerboseFormat(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	res := SiteCheckResult{
		EnvName:     "my-site",
		EnvType:     "prod",
		PluginsNeed: []PluginUpdateInfo{{Name: "akismet", Version: "5.0", UpdateVersion: "5.1", Status: "active"}},
	}
	renderCheckResult(res)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Update Report for: my-site") {
		t.Errorf("expected verbose box header in output, got: %s", out)
	}
}
```

Add these imports to `cmd/check_test.go` if not present:
```go
import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wpengine-cli/internal/ui"
)
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/... -run "TestRunChecksDefaultIsSummaryTable|TestRenderCheckResultVerboseFormat" -v
```
Expected: `FAIL` (functions exist but behavior doesn't match yet)

- [ ] **Step 3: Add `checkVerbose` flag variable**

In `cmd/check.go`, add to the `var` block at the top:
```go
var (
	checkAllEnvs    bool
	checkBatch      string
	checkConcurrent int
	checkMinimal    bool
	checkVerbose    bool // new
)
```

- [ ] **Step 4: Update `runChecks` to flip the default**

In `runChecks`, find the block inside the worker goroutine that currently reads:
```go
} else if checkMinimal {
    countMu.Lock()
    completedCount++
    fmt.Printf("\r\033[K%s Scanned %d/%d environments...", ...)
    countMu.Unlock()
} else {
    printMu.Lock()
    renderCheckResult(res)
    printMu.Unlock()
}
```

Replace with:
```go
} else if checkVerbose {
    printMu.Lock()
    renderCheckResult(res)
    printMu.Unlock()
} else {
    countMu.Lock()
    completedCount++
    count := completedCount
    countMu.Unlock()
    fmt.Printf("\r\033[K%s Scanned %d/%d environments...",
        lipgloss.NewStyle().Foreground(ui.PrimaryColor).Bold(true).Render("●"),
        count, len(resolved))
}
```

Then update the block after `wg.Wait()`:
```go
// Old:
if OutputFormat != "json" && checkMinimal {
    fmt.Print("\r\033[K")
    renderSummaryTable(results)
}

// New:
if OutputFormat != "json" {
    if !checkVerbose {
        fmt.Print("\r\033[K") // clear the progress line
    }
    renderSummaryTable(results)
}
```

- [ ] **Step 5: Replace raw color literals in `check.go` with exported vars**

Search for `lipgloss.Color("214")`, `lipgloss.Color("46")`, `lipgloss.Color("196")`, `lipgloss.Color("39")`, `lipgloss.Color("244")`, `lipgloss.Color("99")` in `cmd/check.go` and replace with `ui.WarningColor`, `ui.SuccessColor`, `ui.ErrorColor`, `ui.InfoColor`, `ui.MutedColor`, `ui.PrimaryColor` respectively.

Verify `"wpengine-cli/internal/ui"` is already in the imports (it is).

- [ ] **Step 6: Register `--verbose` and hide `--minimal` in `init()`**

```go
func init() {
	checkCmd.Flags().StringVar(&checkBatch, "batch", "", "Comma-separated list of environment names, or path to a text file with targets")
	checkCmd.Flags().BoolVar(&checkAllEnvs, "all-envs", false, "Check all active environments under the account")
	checkCmd.Flags().IntVar(&checkConcurrent, "concurrency", 0, "Concurrency limit for checks (falls back to config batch_concurrency)")
	checkCmd.Flags().BoolVar(&checkVerbose, "verbose", false, "Show detailed per-environment update report for each result")
	checkCmd.Flags().BoolVarP(&checkMinimal, "minimal", "m", false, "") // deprecated, now default behavior
	checkCmd.Flags().MarkHidden("minimal")

	RootCmd.AddCommand(checkCmd)
}
```

- [ ] **Step 7: Run the tests to verify they pass**

```bash
go test ./cmd/... -run "TestRunChecksDefaultIsSummaryTable|TestRenderCheckResultVerboseFormat|TestRenderSummaryTable|TestCheckCachingAndHook" -v
```
Expected: all `PASS`

- [ ] **Step 8: Build to verify**

```bash
go build ./...
```

- [ ] **Step 9: Commit**

```bash
git add cmd/check.go cmd/check_test.go
git commit -m "feat: make summary table the default check output, add --verbose for per-env detail"
```

---

## Task 5: `site list` — Spinner + Grouped Tree Layout

**Files:**
- Modify: `cmd/site.go`

- [ ] **Step 1: Add spinner around the API calls**

In `siteListCmd.RunE`, wrap both `GetAllSites()` and `GetAllInstalls()` calls with `ui.RunWithSpinner`. Replace the existing calls:

```go
// Old:
sites, err := APIClient.GetAllSites()
if err != nil {
    return fmt.Errorf("failed to fetch sites: %w", err)
}
// ...
installs, err := APIClient.GetAllInstalls()
if err != nil {
    return fmt.Errorf("failed to fetch environments: %w", err)
}

// New:
var sites []api.Site
err := ui.RunWithSpinner("Fetching sites...", PlainOutput, func() error {
    var e error
    sites, e = APIClient.GetAllSites()
    return e
})
if err != nil {
    return fmt.Errorf("failed to fetch sites: %w", err)
}

var installs []api.Install
err = ui.RunWithSpinner("Fetching environments...", PlainOutput, func() error {
    var e error
    installs, e = APIClient.GetAllInstalls()
    return e
})
if err != nil {
    return fmt.Errorf("failed to fetch environments: %w", err)
}
```

Update the `err` declaration: since we now use `:=` for the spinner calls, change the first one to use `var err error` before or use `=` for subsequent ones. The exact refactor:

```go
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
    // ... rest of function
```

- [ ] **Step 2: Replace the table renderer with a tree renderer**

Remove the block from `fmt.Println("\n" + PrimaryStyle.Render("WP Engine Sites") + "\n")` through `fmt.Printf("\nTotal sites found: %d\n\n", len(filteredSites))` and replace with a call to the new `renderSiteTree` function:

```go
renderSiteTree(filteredSites, siteInstalls, PrimaryStyle)
```

- [ ] **Step 3: Add `renderSiteTree` and `siteEnvBadge` functions to `cmd/site.go`**

Add after the command definitions:

```go
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
```

- [ ] **Step 4: Update imports in `cmd/site.go`**

Add `"strings"` and `"wpengine-cli/internal/ui"` if not already present. Remove `"github.com/charmbracelet/lipgloss/table"` and `"golang.org/x/term"` if they are no longer used (the table and width detection are removed).

```go
import (
	"fmt"
	"strings"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ui"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 5: Build to verify**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/site.go
git commit -m "feat: replace site list table with grouped tree layout and add loading spinner"
```

---

## Task 6: `env list` — Spinner + Grouped Sort

**Files:**
- Modify: `cmd/env.go`

- [ ] **Step 1: Add `"sort"` to imports in `cmd/env.go`**

```go
import (
	"fmt"
	"os"
	"sort"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/config"
	"wpengine-cli/internal/ui"
	"wpengine-cli/internal/ux"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)
```

- [ ] **Step 2: Wrap `GetAllInstalls()` with a spinner**

In `envListCmd.RunE`, replace:
```go
installs, err := APIClient.GetAllInstalls()
if err != nil {
    return fmt.Errorf("failed to fetch environments: %w", err)
}
```

With:
```go
var installs []api.Install
if err := ui.RunWithSpinner("Fetching environments...", PlainOutput, func() error {
    var e error
    installs, e = APIClient.GetAllInstalls()
    return e
}); err != nil {
    return fmt.Errorf("failed to fetch environments: %w", err)
}
```

- [ ] **Step 3: Sort `filteredInstalls` by `Site.ID` before rendering**

After the filtering loop that builds `filteredInstalls`, add:

```go
sort.SliceStable(filteredInstalls, func(i, j int) bool {
	return filteredInstalls[i].Site.ID < filteredInstalls[j].Site.ID
})
```

Place this immediately before the `if envListNames {` block.

- [ ] **Step 4: Build to verify**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/env.go
git commit -m "feat: add loading spinner to env list and sort environments by site grouping"
```

---

## Task 7: Non-Interactive `update` — Final Results Table

**Files:**
- Modify: `cmd/update.go`

- [ ] **Step 1: Replace raw color literals in `update.go` with exported vars**

Search `cmd/update.go` for the inline `lipgloss.Color("...")` literals and replace with the exported vars from the `ui` package (already imported):

| Raw literal | Replace with |
|---|---|
| `lipgloss.Color("46")` | `ui.SuccessColor` |
| `lipgloss.Color("214")` | `ui.WarningColor` |
| `lipgloss.Color("196")` | `ui.ErrorColor` |
| `lipgloss.Color("39")` | `ui.InfoColor` |
| `lipgloss.Color("99")` | `ui.PrimaryColor` |

- [ ] **Step 2: Add `renderUpdateSummaryTable` function to `cmd/update.go`**

Add after the `runNonInteractive` function:

```go
func renderUpdateSummaryTable(jobs []*ui.Job) {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(ui.PrimaryColor)).
		Headers("Environment", "Type", "Status", "Details")

	t.StyleFunc(func(row, col int) lipgloss.Style {
		if row == table.HeaderRow {
			return lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(ui.PrimaryColor).
				Bold(true).
				Padding(0, 1)
		}
		return lipgloss.NewStyle().Padding(0, 1)
	})

	for _, job := range jobs {
		statusStr := lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render("SUCCESS")
		if job.Status == "failed" {
			statusStr = lipgloss.NewStyle().Foreground(ui.ErrorColor).Bold(true).Render("FAILED")
		}

		details := job.Details
		if job.Error != nil {
			details = job.Error.Error()
		}
		if len(details) > 60 {
			details = details[:57] + "..."
		}

		envType := shortEnvType(job.EnvType) // shortEnvType is defined in cmd/check.go, same package
		var typeStr string
		switch envType {
		case "prod":
			typeStr = lipgloss.NewStyle().Foreground(ui.ErrorColor).Bold(true).Render("PROD")
		case "stg":
			typeStr = lipgloss.NewStyle().Foreground(ui.WarningColor).Bold(true).Render("STG")
		default:
			typeStr = lipgloss.NewStyle().Foreground(ui.InfoColor).Bold(true).Render("DEV")
		}

		t.Row(job.Name, typeStr, statusStr, details)
	}

	fmt.Println(t.Render())
}
```

`shortEnvType` is defined in `cmd/check.go` — both files are in the `cmd` package, so it is directly accessible.

- [ ] **Step 3: Call `renderUpdateSummaryTable` at the end of `runNonInteractive`**

In `runNonInteractive`, replace the existing final print block:
```go
// Old:
if OutputFormat != "json" {
    fmt.Println("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(ux.Symbol("check", PlainOutput)+" All operations completed.") + "\n")
}

// New:
if OutputFormat != "json" {
    fmt.Println()
    renderUpdateSummaryTable(jobs)
    fmt.Println(lipgloss.NewStyle().Foreground(ui.SuccessColor).Bold(true).Render(ux.Symbol("check", PlainOutput)+" All operations completed.") + "\n")
}
```

- [ ] **Step 4: Add `"github.com/charmbracelet/lipgloss/table"` import if missing**

Check `cmd/update.go` imports — add `"github.com/charmbracelet/lipgloss/table"` if it is not present.

- [ ] **Step 5: Build to verify**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add cmd/update.go
git commit -m "feat: add final results table to non-interactive update mode"
```

---

## Task 8: `doctor` — Summary Line

**Files:**
- Modify: `cmd/doctor.go`

- [ ] **Step 1: Write the failing test**

Add a new file `cmd/doctor_test.go`:

```go
package cmd

import "testing"

func TestDoctorSummary(t *testing.T) {
	checks := []doctorCheck{
		{Status: "ok"},
		{Status: "ok"},
		{Status: "warn"},
		{Status: "fail"},
	}
	passed, warned, failed := doctorCounts(checks)
	if passed != 2 {
		t.Errorf("expected 2 passed, got %d", passed)
	}
	if warned != 1 {
		t.Errorf("expected 1 warned, got %d", warned)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./cmd/... -run TestDoctorSummary -v
```
Expected: `FAIL — doctorCounts undefined`

- [ ] **Step 3: Add `doctorCounts` helper and summary output to `cmd/doctor.go`**

Add the helper function:

```go
func doctorCounts(checks []doctorCheck) (passed, warned, failed int) {
	for _, c := range checks {
		switch c.Status {
		case "ok":
			passed++
		case "warn":
			warned++
		case "fail":
			failed++
		}
	}
	return
}
```

In `doctorCmd.RunE`, after the `for _, check := range checks { ... }` print loop and `fmt.Println()`, add the summary line:

```go
passed, warned, failed := doctorCounts(checks)
if PlainOutput {
    fmt.Printf("%d passed · %d warnings · %d failures\n\n", passed, warned, failed)
} else {
    passedStr := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(fmt.Sprintf("%d passed", passed))
    warnStr := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render(fmt.Sprintf("%d warnings", warned))
    failStr := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(fmt.Sprintf("%d failures", failed))
    fmt.Printf("%s · %s · %s\n\n", passedStr, warnStr, failStr)
}
```

Place this before the `if doctorHasFailure(checks) { return ... }` check.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./cmd/... -run TestDoctorSummary -v
```
Expected: `PASS`

- [ ] **Step 5: Build to verify**

```bash
go build ./...
```

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```
Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/doctor.go cmd/doctor_test.go
git commit -m "feat: add summary line to doctor output"
```

---

## Task 9: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the `check` section**

Find the `### 6. Check for Outstanding Updates` section and replace the `--minimal` flag documentation with `--verbose`:

```markdown
**Verbose Output**
To show detailed per-environment update reports as results arrive (useful when checking a single site or debugging):
```bash
./wpengine check my-dev-sandbox --verbose
```

Remove:
```markdown
**Minimal Output Flag**
To minimize verbose output and display a summary table of the environments' update status:
```bash
./wpengine check --all-envs --minimal
# or
./wpengine check --all-envs -m
```
```

- [ ] **Step 2: Update the `site list` section**

Replace:
```markdown
List all top-level sites under your account (including a column displaying their associated environments):
```

With:
```markdown
List all top-level sites under your account. Sites are shown as headers with their prod/stg/dev environments nested below, making it easy to see which environments belong to each site:
```

- [ ] **Step 3: Update the Architecture section**

Replace the `internal/ui/` line:
```markdown
- `internal/ui/`: Bubble Tea and Lipgloss CLI components.
```

With:
```markdown
- `internal/ui/`: Bubble Tea and Lipgloss CLI components (`dashboard.go`), shared color palette (`styles.go`), and reusable loading spinner (`spinner.go`).
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: update README for check --verbose, site list tree layout, and ui architecture"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** All seven spec sections have corresponding tasks (shared styles → T1, spinner → T2, dashboard elapsed → T3, check default → T4, site list tree → T5, env list sort → T6, update summary table → T7, doctor summary → T8). README → T9.
- [x] **No placeholders:** All steps contain actual code or exact commands.
- [x] **Type consistency:** `PrimaryColor`, `SuccessColor`, `WarningColor`, `ErrorColor`, `InfoColor`, `MutedColor` defined in T1 and used by name consistently in T2–T8. `Job.StartedAt` defined in T3 and read in T3's View() update. `renderUpdateSummaryTable` defined and called in T7. `doctorCounts` defined and tested in T8.
- [x] **`shortEnvType` cross-file use:** Noted in T7 — it lives in `cmd/check.go`, accessible from `cmd/update.go` since both are in `package cmd`.
- [x] **`--minimal` deprecation:** Hidden via `MarkHidden` in T4, not removed, so existing scripts continue to work.
- [x] **`--verbose` on `check`:** Registered as a local flag (not a shorthand) to avoid collision with root's `-v, --verbose` persistent flag.

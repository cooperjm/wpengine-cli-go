# UI/UX Improvements — Design Spec

**Date:** 2026-05-26  
**Status:** Approved  
**Scope:** Approach 2 — Targeted fixes + consistency pass

---

## Problem Statement

The CLI has several readability and usability gaps identified during a UX review:

1. `site list` packs all environments into a single comma-joined cell, making it hard to distinguish prod/stg/dev at a glance.
2. `site list` and `env list` hang silently during API calls with no loading feedback.
3. `check` defaults to a verbose per-environment box layout; the compact summary table is hidden behind `--minimal` and rarely discovered.
4. Colors and styles are hardcoded as raw `lipgloss.Color` values in `check.go` and `update.go` instead of using the named palette defined in `dashboard.go`.
5. The update TUI dashboard shows no timing information per job or overall.
6. `doctor` has no summary line — you have to count results yourself.
7. Non-interactive `update` mode streams logs but produces no final summary, making CI log scanning harder.

---

## Out of Scope (Future Work)

**Interactive home screen TUI** — Running `wpengine` with no args would launch a menu-driven TUI for navigating sites, triggering checks, and running updates. This is explicitly queued as a follow-up project after this UX refresh ships.

---

## Architecture

Changes are distributed across six areas:

| File(s) | Change |
|---|---|
| `internal/ui/styles.go` (new) | Shared color palette and style definitions |
| `internal/ui/spinner.go` (new) | Reusable single-spinner Bubble Tea program |
| `cmd/site.go` | Spinner + grouped tree layout for `site list` |
| `cmd/env.go` | Spinner for `env list`; grouped sort order |
| `cmd/check.go` | Summary table as default; `--verbose` flag; improved progress line |
| `cmd/update.go` + `internal/ui/dashboard.go` | Elapsed time per job + total; final summary table in non-interactive mode |
| `cmd/doctor.go` | Summary line at end of output |

---

## Section 1: Shared Style Package

**File:** `internal/ui/styles.go`

Extract the color palette from `dashboard.go` into a package-level set of exported constants and styles:

```go
var (
    PrimaryColor = lipgloss.Color("99")   // indigo/purple
    SuccessColor = lipgloss.Color("46")   // emerald green
    WarningColor = lipgloss.Color("214")  // amber
    ErrorColor   = lipgloss.Color("196")  // red
    InfoColor    = lipgloss.Color("39")   // sky blue
    MutedColor   = lipgloss.Color("244")  // slate gray
)
```

All raw color literals in `check.go` and `update.go` are replaced with references to these. No behavior changes — purely a consistency fix that makes future theming easier.

---

## Section 2: Loading Spinner

**File:** `internal/ui/spinner.go`

A minimal Bubble Tea program with a single spinner and a message string. Used by `site list`, `env list`, and `check` to display a loading indicator while waiting for the WP Engine API.

Behavior:
- Starts immediately when the command runs, before the API call begins.
- Shows: `● Fetching environments...` (spinner animates in place).
- Terminates and clears the line as soon as the API call returns.
- Falls back to a no-op when `--plain` is set or stdout is not a terminal.

The spinner is not a full dashboard model — it wraps the API call synchronously and hands control back to the caller with the result.

---

## Section 3: `site list` — Grouped Tree Layout

**File:** `cmd/site.go`

Replace the current Lipgloss table (which packs all environments into one cell) with a custom tree renderer:

**Structure:**
- Each site renders as a bold header line with its name and dimmed UUID.
- Environments are rendered beneath it, indented, one line each.
- Each environment line: colored `PROD` / `STG` / `DEV` badge, install name, CNAME/primary domain, active/inactive status.
- A divider line separates sites.
- Footer: `N sites · M environments`.

**Filtering:** The existing `--production`, `--staging`, `--dev` flags continue to work — a site is hidden entirely if none of its environments match the filter.

**`env list` change:** Stays as a flat table with its existing columns. The sort order changes to group by `Site.ID` so all environments belonging to the same site appear consecutively — no extra API call needed since `Install.Site.ID` is already in the response.

---

## Section 4: `check` Output

**File:** `cmd/check.go`

### Default behavior (was `--minimal`)

The summary table is now the default output. Scanning progress is shown as:

```
● Scanning 12/20 environments...
```

Updated in-place as each result arrives (replaces the raw `\r\033[K` escape with a proper counter driven by the same atomic counter already in the code).

Once all checks complete, the table is printed and the progress line is cleared.

### `--verbose` flag (replaces current default)

Shows the full per-environment bordered box as each result arrives, then prints the summary table at the end.

### `--minimal` flag

Kept as a hidden, deprecated alias for the default behavior. Scripts using `--minimal` continue to work without changes; the flag just becomes a no-op since the summary table is now the default.

### Error display

Errors in the summary table are shown as a short message. `--verbose` shows the full error in the box.

---

## Section 5: Update Dashboard — Elapsed Time

**Files:** `internal/ui/dashboard.go`, `cmd/update.go`

### Interactive TUI

Each `Job` gains a `StartedAt time.Time` field, set when the job leaves `idle` status. The dashboard `View()` renders a duration next to each status badge:

```
⠸  BACKING UP  acme-production   Polling backup…     (0m 28s)
✔  SUCCESS     globex-wp         Plugins updated      (1m 42s)
```

The dashboard header adds a total elapsed counter:

```
WP Engine Site Update Dashboard    Elapsed: 2m 14s
```

Updated on every spinner tick, so it animates smoothly.

### Non-interactive mode

After all jobs complete, print a final results table:

| Environment | Type | Status | Duration | Details |
|---|---|---|---|---|
| acme-production | PROD | SUCCESS | 1m 42s | Plugins updated |
| globex-wp | PROD | FAILED | 0m 12s | SSH connection failed |

Same Lipgloss table style as the `check` summary table.

---

## Section 6: `doctor` Summary Line

**File:** `cmd/doctor.go`

After printing all check rows, add:

```
3 checks passed · 1 warning · 0 failures
```

Colored: green count for passed, amber for warnings, red for failures. In `--plain` mode, plain text. In `--output json`, the summary is not added (JSON output is already structured).

---

## Error Handling

No new error conditions are introduced. The spinner handles API errors by terminating and passing the error back to the calling command, which handles it the same way it does today.

---

## Testing

- `internal/ui/styles.go`: No tests needed — it's constants.
- `internal/ui/spinner.go`: Covered by existing spinner behavior; a unit test verifying the plain-mode no-op is sufficient.
- `cmd/check.go`: Update `check_test.go` to assert the summary table is produced by default and the `--verbose` flag produces the box output.
- `cmd/doctor.go`: Add a test asserting the summary line counts match the check results.
- Visual/TUI changes (dashboard, site list): Manual verification.

---

## Future Work

- **Interactive home screen TUI** (`wpengine` with no args launches a menu): queued as a separate project, to be brainstormed after this ships.

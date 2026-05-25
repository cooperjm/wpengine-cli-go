package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"wpengine-cli/internal/api"
	"wpengine-cli/internal/ssh"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// UI Color Palette and Lipgloss Styles
var (
	primaryColor = lipgloss.Color("99")  // Deep Indigo/Purple
	successColor = lipgloss.Color("46")  // Emerald Green
	warningColor = lipgloss.Color("214") // Amber/Yellow
	errorColor   = lipgloss.Color("196") // Rose/Red
	infoColor    = lipgloss.Color("39")  // Sky Blue
	mutedColor   = lipgloss.Color("244") // Slate Gray

	// Style Definitions
	titleStyle = lipgloss.NewStyle().
			Background(primaryColor).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1).
			Bold(true)

	boldStyle = lipgloss.NewStyle().Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2).
			Margin(1, 0)

	bannerStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Padding(0, 1)
)

// GetStatusBadge returns a lipgloss styled badge for the job status.
func GetStatusBadge(status string) string {
	switch status {
	case "idle":
		return lipgloss.NewStyle().Background(mutedColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" PENDING ")
	case "verifying_ssh":
		return lipgloss.NewStyle().Background(infoColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" VERIFY SSH ")
	case "backing_up":
		return lipgloss.NewStyle().Background(warningColor).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(" BACKUP INITIATED ")
	case "polling_backup":
		return lipgloss.NewStyle().Background(warningColor).Foreground(lipgloss.Color("232")).Bold(true).Padding(0, 1).Render(" BACKING UP ")
	case "updating":
		return lipgloss.NewStyle().Background(primaryColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" UPDATING ")
	case "completed":
		return lipgloss.NewStyle().Background(successColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" SUCCESS ")
	case "failed":
		return lipgloss.NewStyle().Background(errorColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" FAILED ")
	default:
		return lipgloss.NewStyle().Background(mutedColor).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1).Render(" " + strings.ToUpper(status) + " ")
	}
}

// PrintLog prints a styled message for non-interactive output (CI/CD environments).
func PrintLog(badge, name, message string, color lipgloss.Color) {
	badgeStyle := lipgloss.NewStyle().Background(color).Foreground(lipgloss.Color("255")).Bold(true).Padding(0, 1)
	nameStyle := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	fmt.Printf("%s %s: %s\n", badgeStyle.Render(badge), nameStyle.Render(name), message)
}

// Job represents a single site update task.
type Job struct {
	ID      string
	Name    string
	Status  string // idle, verifying_ssh, backing_up, polling_backup, updating, completed, failed
	Details string
	Error   error
}

// Bubble Tea Model for Interactive Dashboard
type Model struct {
	Jobs         []*Job
	client       *api.Client
	sshClient    *ssh.Client
	scope        string
	dryRun       bool
	concurrency  int
	spinner      spinner.Model
	msgChan      chan tea.Msg
	done         bool
	quitting     bool
	wg           sync.WaitGroup
	mu           sync.Mutex
}

// JobUpdateMsg is sent when a job's status updates.
type JobUpdateMsg struct {
	Index   int
	Status  string
	Details string
	Err     error
}

// FinishedMsg is sent when all jobs have processed.
type FinishedMsg struct{}

// NewModel creates a new Bubble Tea model.
func NewModel(jobs []*Job, client *api.Client, sshClient *ssh.Client, scope string, dryRun bool, concurrency int) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	return &Model{
		Jobs:        jobs,
		client:      client,
		sshClient:   sshClient,
		scope:       scope,
		dryRun:      dryRun,
		concurrency: concurrency,
		spinner:     s,
		msgChan:     make(chan tea.Msg, len(jobs)*5),
	}
}

// Init initializes the Bubble Tea program.
func (m *Model) Init() tea.Cmd {
	// Start workers in background
	m.startWorkers()

	return tea.Batch(
		m.spinner.Tick,
		m.recvCmd(),
	)
}

// recvCmd listens for messages on the background channel.
func (m *Model) recvCmd() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.msgChan
		if !ok {
			return FinishedMsg{}
		}
		return msg
	}
}

// Update processes Bubble Tea messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case JobUpdateMsg:
		m.mu.Lock()
		job := m.Jobs[msg.Index]
		job.Status = msg.Status
		job.Details = msg.Details
		job.Error = msg.Err
		m.mu.Unlock()
		return m, m.recvCmd()

	case FinishedMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// View renders the Bubble Tea UI.
func (m *Model) View() string {
	var sb strings.Builder

	// Title Banner
	sb.WriteString("\n")
	sb.WriteString(titleStyle.Render(" WP Engine Site Update Dashboard "))
	sb.WriteString("\n\n")

	// Job List
	for i, job := range m.Jobs {
		m.mu.Lock()
		status := job.Status
		details := job.Details
		err := job.Error
		name := job.Name
		m.mu.Unlock()

		badge := GetStatusBadge(status)
		
		var statusText string
		if status == "failed" && err != nil {
			statusText = lipgloss.NewStyle().Foreground(errorColor).Render(err.Error())
		} else {
			statusText = lipgloss.NewStyle().Foreground(mutedColor).Render(details)
		}

		// Add spinner if job is active
		spinStr := "  "
		if status != "idle" && status != "completed" && status != "failed" {
			spinStr = m.spinner.View() + " "
		}

		sb.WriteString(fmt.Sprintf("%s %s %s\n", spinStr, badge, boldStyle.Render(name)))
		if statusText != "" {
			sb.WriteString(fmt.Sprintf("     └─ %s\n", statusText))
		}
		if i < len(m.Jobs)-1 {
			sb.WriteString("\n")
		}
	}

	if m.done {
		sb.WriteString("\n" + lipgloss.NewStyle().Foreground(successColor).Bold(true).Render("✔ All operations completed.") + "\n")
	} else if m.quitting {
		sb.WriteString("\n" + lipgloss.NewStyle().Foreground(errorColor).Bold(true).Render("✖ Terminated by user.") + "\n")
	} else {
		sb.WriteString("\nPress 'q' or Ctrl+C to quit.\n")
	}

	return boxStyle.Render(sb.String())
}

// startWorkers launches the worker pool to execute backup & update tasks.
func (m *Model) startWorkers() {
	jobChan := make(chan int, len(m.Jobs))
	for i := range m.Jobs {
		jobChan <- i
	}
	close(jobChan)

	// Concurrency safety
	sem := make(chan struct{}, m.concurrency)

	for i := 0; i < m.concurrency; i++ {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for idx := range jobChan {
				sem <- struct{}{} // Acquire semaphore
				m.runJob(idx)
				<-sem // Release semaphore
			}
		}()
	}

	// Wait in background and close the message channel when done
	go func() {
		m.wg.Wait()
		close(m.msgChan)
	}()
}

// runJob handles the execution sequence for a single job: verify, backup, poll, update.
func (m *Model) runJob(idx int) {
	job := m.Jobs[idx]

	// 1. Verify SSH Connection
	m.msgChan <- JobUpdateMsg{Index: idx, Status: "verifying_ssh", Details: "Testing SSH Gateway connection..."}
	err := m.sshClient.VerifyConnection(job.Name)
	if err != nil {
		m.msgChan <- JobUpdateMsg{Index: idx, Status: "failed", Err: fmt.Errorf("SSH connection verification failed: %w", err)}
		return
	}

	// 2. Trigger Backup
	m.msgChan <- JobUpdateMsg{Index: idx, Status: "backing_up", Details: "Requesting API backup checkpoint..."}
	backupDesc := fmt.Sprintf("cli-pre-update-%d", time.Now().Unix())
	backup, err := m.client.CreateBackup(job.ID, backupDesc)
	if err != nil {
		m.msgChan <- JobUpdateMsg{Index: idx, Status: "failed", Err: fmt.Errorf("failed to trigger backup: %w", err)}
		return
	}

	// 3. Poll Backup
	m.msgChan <- JobUpdateMsg{Index: idx, Status: "polling_backup", Details: fmt.Sprintf("Waiting for backup %s (status: %s)", backup.ID, backup.Status)}
	statusChan, errChan := m.client.PollBackupStatus(job.ID, backup.ID, 8*time.Second, 15*time.Minute)

pollLoop:
	for {
		select {
		case b, ok := <-statusChan:
			if !ok {
				break pollLoop
			}
			m.msgChan <- JobUpdateMsg{
				Index:   idx,
				Status:  "polling_backup",
				Details: fmt.Sprintf("Backup status: %s", b.Status),
			}
		case err := <-errChan:
			if err != nil {
				m.msgChan <- JobUpdateMsg{Index: idx, Status: "failed", Err: fmt.Errorf("backup failed: %w", err)}
				return
			}
		}
	}

	// 4. Run Update via SSH
	m.msgChan <- JobUpdateMsg{Index: idx, Status: "updating", Details: "Executing updates via WP-CLI SSH..."}

	wpArgs := []string{"plugin", "update"}
	if m.dryRun {
		wpArgs = append(wpArgs, "--dry-run")
	}

	switch m.scope {
	case "plugins":
		wpArgs = append(wpArgs, "--all")
	case "themes":
		// wp theme update --all
		wpArgs[0] = "theme"
		wpArgs = append(wpArgs, "--all")
	case "core":
		// wp core update
		wpArgs[0] = "core"
	case "all":
		// We'll update plugins first, then themes, then core
		// Let's run a combined script or sequentially. Sequentially is safer.
		scopes := []string{"plugin", "theme", "core"}
		for _, s := range scopes {
			args := []string{s, "update"}
			if m.dryRun {
				args = append(args, "--dry-run")
			}
			if s != "core" {
				args = append(args, "--all")
			}
			m.msgChan <- JobUpdateMsg{Index: idx, Status: "updating", Details: fmt.Sprintf("Executing: wp %s update", s)}
			stdout, stderr, err := m.sshClient.RunWPCLI(job.Name, args...)
			if err != nil {
				m.msgChan <- JobUpdateMsg{Index: idx, Status: "failed", Err: fmt.Errorf("wp %s update failed: %w (stderr: %s)", s, err, stderr)}
				return
			}
			job.Details = stdout
		}
		m.msgChan <- JobUpdateMsg{Index: idx, Status: "completed", Details: "WordPress core, plugins, and themes successfully updated."}
		return
	}

	stdout, stderr, err := m.sshClient.RunWPCLI(job.Name, wpArgs...)
	if err != nil {
		m.msgChan <- JobUpdateMsg{Index: idx, Status: "failed", Err: fmt.Errorf("wp-cli update failed: %w (stderr: %s)", err, stderr)}
		return
	}

	summary := stdout
	if summary == "" {
		summary = "No updates available or completed successfully."
	} else if len(summary) > 60 {
		// Truncate long outputs slightly for visual layout
		lines := strings.Split(summary, "\n")
		summary = lines[len(lines)-1]
	}

	m.msgChan <- JobUpdateMsg{Index: idx, Status: "completed", Details: summary}
}

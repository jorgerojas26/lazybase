package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"lazybase/internal/ports"
	"lazybase/internal/registry"
	"lazybase/internal/supabase"
)

type statusMsg struct {
	index  int
	status supabase.Status
	err    error
}

type model struct {
	store    *registry.Store
	reg      *registry.Registry
	projects []registry.Project
	settings ports.Settings
	cursor   int
	statuses []supabase.Status
	message  string
	quitting bool
}

func Run(store *registry.Store, settings ports.Settings) error {
	reg, err := store.Load()
	if err != nil {
		return err
	}

	projects := store.List(reg)
	m := model{
		store:    store,
		reg:      reg,
		projects: projects,
		settings: settings,
		statuses: make([]supabase.Status, len(projects)),
	}

	program := tea.NewProgram(m)
	_, err = program.Run()
	return err
}

func (m model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.projects))
	for i, project := range m.projects {
		index := i
		path := project.Path
		cmds = append(cmds, func() tea.Msg {
			status, err := supabase.StatusForProject(path)
			return statusMsg{index: index, status: status, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "enter", "o":
			if len(m.projects) == 0 {
				break
			}
			url := m.selectedStudioURL()
			if url == "" {
				m.message = "No Studio URL available"
				break
			}
			if err := openURL(url); err != nil {
				m.message = fmt.Sprintf("Open failed: %v", err)
			} else {
				m.message = "Opened Studio"
			}
		case "p":
			if len(m.projects) == 0 {
				break
			}
			path := m.projects[m.cursor].Path
			if !m.store.Prune(m.reg, path) {
				break
			}
			if err := m.store.Save(m.reg); err != nil {
				m.message = fmt.Sprintf("Prune failed: %v", err)
				break
			}
			m.projects = m.store.List(m.reg)
			m.statuses = resizeStatuses(m.statuses, len(m.projects))
			if m.cursor >= len(m.projects) && m.cursor > 0 {
				m.cursor--
			}
			m.message = "Pruned registry entry"
		}
	case statusMsg:
		if msg.index >= 0 && msg.index < len(m.statuses) {
			if msg.err != nil {
				m.statuses[msg.index] = supabase.Status{State: "unknown", Details: msg.err.Error()}
			} else {
				m.statuses[msg.index] = msg.status
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString("Lazybase\n\n")
	if len(m.projects) == 0 {
		b.WriteString("No registered projects. Run `lazybase start` inside a Supabase project first.\n")
		if m.message != "" {
			b.WriteString("\n" + m.message + "\n")
		}
		b.WriteString("\nq quit\n")
		return b.String()
	}

	b.WriteString("Name               Path                                     Slot  Ports        Status    Studio\n")
	b.WriteString("------------------------------------------------------------------------------------------------\n")
	for i, project := range m.projects {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}

		portMap := ports.Compute(m.settings, project.Slot, ports.DisplayKeys())
		status := m.statuses[i].State
		if status == "" {
			status = "loading"
		}
		studio := m.statuses[i].StudioURL
		if studio == "" {
			studio = ports.StudioURL(portMap)
		}

		line := fmt.Sprintf(
			"%s %-16s %-40s %-4d  %-10s  %-8s  %s\n",
			cursor,
			truncate(filepath.Base(project.Path), 16),
			truncate(project.Path, 40),
			project.Slot,
			ports.RangeSummary(portMap),
			truncate(status, 8),
			studio,
		)
		b.WriteString(line)
	}

	if m.message != "" {
		b.WriteString("\n" + m.message + "\n")
	}

	b.WriteString("\nenter/o open studio • p prune entry • q quit\n")
	return b.String()
}

func (m model) selectedStudioURL() string {
	if len(m.projects) == 0 {
		return ""
	}
	if url := m.statuses[m.cursor].StudioURL; url != "" {
		return url
	}
	portMap := ports.Compute(m.settings, m.projects[m.cursor].Slot, ports.DisplayKeys())
	return ports.StudioURL(portMap)
}

func resizeStatuses(statuses []supabase.Status, length int) []supabase.Status {
	resized := make([]supabase.Status, length)
	copy(resized, statuses)
	return resized
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

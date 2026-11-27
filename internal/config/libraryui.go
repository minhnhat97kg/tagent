package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"tagent/internal/library"
	"tagent/internal/tasks"
)

type libraryEditedMsg struct{ err error }

func (m uiModel) libraryBody(dim, accent lipgloss.Style) []string {
	root := tasks.MainRoot(workDir())
	lib, err := library.Load(root)
	if err != nil {
		return []string{accent.Render("Library error"), err.Error()}
	}
	lines := []string{accent.Render("Repository library · edits are shared with this board"), "Use Board tab to select skills/MCP per role in board.yaml.", ""}
	var names []string
	if m.tab == tabSkills {
		for n := range lib.Skills {
			names = append(names, n)
		}
	} else {
		for n := range lib.Servers {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		lines = append(lines, dim.Render("No entries. Press e to create/edit the library manifest."))
	}
	for i, name := range names {
		prefix := "  "
		if i == m.libraryIndex {
			prefix = "> "
		}
		if m.tab == tabSkills {
			lines = append(lines, prefix+name+dim.Render(" · library"))
			continue
		}
		s, _ := library.Approved(approvalDir(), lib.Servers[name])
		status := "unapproved"
		if s {
			status = "approved"
		}
		server := lib.Servers[name]
		lines = append(lines, prefix+name+dim.Render(" · "+status+" · "+server.Command+" "+fmt.Sprint(server.Args)+" env="+fmt.Sprint(server.Env)))
	}
	return lines
}

func nextSkillPath(root string) string {
	base := filepath.Join(root, ".tasks", "library", "skills")
	for i := 1; ; i++ {
		path := filepath.Join(base, fmt.Sprintf("skill-%d", i), "SKILL.md")
		if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
			return path
		}
	}
}

func approvalDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "tagent")
}

func (m uiModel) editLibrary() (uiModel, tea.Cmd) {
	root := tasks.MainRoot(workDir())
	var path string
	if m.tab == tabMCP {
		path = filepath.Join(root, ".tasks", "library", "mcp.yml")
	} else {
		lib, _ := library.Load(root)
		var names []string
		for n := range lib.Skills {
			names = append(names, n)
		}
		sort.Strings(names)
		if len(names) == 0 {
			path = nextSkillPath(root)
		} else {
			if m.libraryIndex >= len(names) {
				m.libraryIndex = len(names) - 1
			}
			path = filepath.Join(lib.Skills[names[m.libraryIndex]].Path, "SKILL.md")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		m.ed.SetFlash(err.Error(), true)
		return m, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) && m.tab == tabSkills {
		if err = os.WriteFile(path, []byte("---\nname: "+filepath.Base(filepath.Dir(path))+"\n---\n\n# Skill\n"), 0644); err != nil {
			m.ed.SetFlash(err.Error(), true)
			return m, nil
		}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) && m.tab == tabMCP {
		if err = os.WriteFile(path, []byte("servers: []\n"), 0644); err != nil {
			m.ed.SetFlash(err.Error(), true)
			return m, nil
		}
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return libraryEditedMsg{err} })
}

func (m uiModel) approveLibraryServer() (uiModel, tea.Cmd) {
	lib, err := library.Load(tasks.MainRoot(workDir()))
	if err != nil {
		m.ed.SetFlash(err.Error(), true)
		return m, nil
	}
	var names []string
	for n := range lib.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == 0 {
		m.ed.SetFlash("no MCP servers to approve", true)
		return m, nil
	}
	if m.libraryIndex >= len(names) {
		m.libraryIndex = len(names) - 1
	}
	name := names[m.libraryIndex]
	if err := library.Approve(approvalDir(), lib.Servers[name]); err != nil {
		m.ed.SetFlash(err.Error(), true)
	} else {
		m.ed.SetFlash(fmt.Sprintf("approved MCP server %s", name), false)
	}
	return m, nil
}

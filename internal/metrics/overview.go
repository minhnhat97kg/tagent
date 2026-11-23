package metrics

import (
	"fmt"
	"strings"
	"time"
	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

type Day struct {
	Date   string `json:"date"`
	Tokens int64  `json:"tokens"`
}
type Window struct {
	Today     int64  `json:"today"`
	Days      []Day  `json:"days"`
	Total     int64  `json:"total7d"`
	Sparkline string `json:"sparkline"`
}

func TokenWindow(s State, now time.Time, days int) Window {
	w := Window{Days: []Day{}}
	var peak int64
	for i := days - 1; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		var n int64
		for _, r := range s.Sessions {
			n += r.Daily[d]
		}
		w.Days = append(w.Days, Day{d, n})
		w.Total += n
		if n > peak {
			peak = n
		}
		if i == 0 {
			w.Today = n
		}
	}
	chars := []rune("▁▂▃▄▅▆▇█")
	for _, d := range w.Days {
		if peak == 0 {
			w.Sparkline += "·"
		} else {
			w.Sparkline += string(chars[min(7, int(d.Tokens*7/peak))])
		}
	}
	return w
}

type AttentionItem struct {
	Kind  string  `json:"kind"`
	Text  string  `json:"text"`
	Pane  *string `json:"pane"`
	Count int     `json:"count"`
}
type Attention struct {
	Needed bool            `json:"needed"`
	Text   string          `json:"text"`
	Items  []AttentionItem `json:"items"`
}

func OverviewFacts(panes []tmux.PaneInfo, tickets []tasks.Task, state State) (Attention, []string) {
	a := Attention{Text: "nothing needs you", Items: []AttentionItem{}}
	now := []string{}
	for _, p := range panes {
		label := p.Name
		if label == "" {
			label = string(p.Agent)
		}
		kind, text := "", ""
		if p.Status == tmux.StatusWaiting {
			kind, text = "waiting", label+" waiting"
		}
		if p.Status == tmux.StatusRunning && p.Stuck != "" {
			kind, text = "quiet", label+" quiet "+p.Stuck+"m"
		}
		if kind != "" {
			id := p.PaneID
			a.Items = append(a.Items, AttentionItem{kind, text, &id, 1})
		}
	}
	blocked := 0
	for _, t := range tickets {
		if t.Status == "blocked" {
			blocked++
		}
		if t.Status != "doing" {
			continue
		}
		who := ""
		for _, r := range state.Runs {
			if r.TaskID == t.ID && r.Ended.IsZero() {
				who = r.Agent + ": "
				break
			}
		}
		now = append(now, who+strings.ReplaceAll(t.Text, "\n", " "))
	}
	if blocked > 0 {
		a.Items = append(a.Items, AttentionItem{"blocked", fmt.Sprintf("%d blocked", blocked), nil, blocked})
	}
	if len(a.Items) > 0 {
		a.Needed = true
		parts := []string{}
		for _, i := range a.Items {
			parts = append(parts, i.Text)
		}
		a.Text = strings.Join(parts, " · ")
	}
	return a, now
}

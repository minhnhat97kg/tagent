package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CapText bounds agent context by Unicode characters, not bytes.
func CapText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "… see task show --all"
}
func Brief(t Task) string {
	b := fmt.Sprintf("%s: %s\nStatus: %s\n%s\n", t.ID, t.Title, t.Status, CapText(t.Text, 4000))
	if t.Priority != "" {
		b += "Priority: " + NormalizePriority(t.Priority) + "\n"
	}
	if len(t.Needs) > 0 {
		b += "Waits for: " + strings.Join(t.Needs, ", ") + "\n"
	}
	data, _ := json.Marshal(t.Outputs)
	b += "Outputs: " + string(data) + "\n"
	var cs []TaskComment
	for _, c := range t.Comments {
		if c.Author != "system" {
			cs = append(cs, c)
		}
	}
	if len(cs) > 3 {
		cs = cs[len(cs)-3:]
	}
	for _, c := range cs {
		b += CommentBrief(c)
	}
	return b
}
func CommentBrief(c TaskComment) string {
	b := fmt.Sprintf("[%s] %s: %s\n", c.ID, c.Author, CapText(c.Body, 600))
	for _, a := range c.Attachments {
		b += "Attachment: " + a.File + "\n"
	}
	return b
}

func moveAttachments(scope, id string, c *TaskComment, paths []string) error {
	start := len(c.Attachments)
	success := false
	defer func() {
		if !success {
			for _, a := range c.Attachments[start:] {
				os.Remove(filepath.Join(boardDir(scope), id, a.File))
			}
			c.Attachments = c.Attachments[:start]
		}
	}()
	for i, p := range paths {
		info, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("attachment must be regular")
		}
		in, e := os.Open(p)
		if e != nil {
			return e
		}
		name := filepath.Base(p)
		file := fmt.Sprintf("%s-%d-%s", c.ID, i, name)
		e = storeAttachment(scope, id, file, in)
		in.Close()
		if e != nil {
			return e
		}
		c.Attachments = append(c.Attachments, Attachment{Name: name, File: file})
	}
	success = true
	return nil
}

// compactBody runs under the ticket lock; attachment names are unique per comment.
func compactBody(scope, id string, c *TaskComment) error {
	if len([]rune(c.Body)) <= 600 {
		return nil
	}
	name := c.ID + "-comment.md"
	if e := storeAttachment(scope, id, name, strings.NewReader(c.Body)); e != nil {
		return e
	}
	c.Attachments = append(c.Attachments, Attachment{Name: "comment.md", File: name})
	lines := strings.Split(string([]rune(c.Body)[:500]), "\n")
	c.Body = strings.Join(lines[:min(3, len(lines))], "\n") + "\n(full text attached)"
	return nil
}

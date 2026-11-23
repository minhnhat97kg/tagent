package tasks

import (
	"encoding/json"
	"fmt"
	"io"
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
	dir := filepath.Join(boardDir(scope), id)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid attachment directory")
	}
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
		out, e := os.OpenFile(filepath.Join(dir, file), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return e
		}
		_, e = io.Copy(out, in)
		in.Close()
		ce := out.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		c.Attachments = append(c.Attachments, Attachment{Name: name, File: file})
	}
	return nil
}

// compactBody runs under the ticket lock; attachment names are unique per comment.
func compactBody(scope, id string, c *TaskComment) error {
	if len([]rune(c.Body)) <= 600 {
		return nil
	}
	dir := filepath.Join(boardDir(scope), id)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid attachment directory")
	}
	name := c.ID + "-comment.md"
	f, e := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.WriteString(c.Body)
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	c.Attachments = append(c.Attachments, Attachment{Name: "comment.md", File: name})
	lines := strings.Split(string([]rune(c.Body)[:500]), "\n")
	c.Body = strings.Join(lines[:min(3, len(lines))], "\n") + "\n(full text attached)"
	return nil
}

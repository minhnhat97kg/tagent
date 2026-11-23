package intake

import (
	"regexp"
	"strings"
)

type Completion struct{ Token, Label string }

var addressSuffix = regexp.MustCompile(`(?:^|\s)@([^\s` + "`" + `]*)$`)

func Completions(input string, recipients []Recipient) []Completion {
	loc := addressSuffix.FindStringSubmatch(input)
	if loc == nil || strings.Count(input, "`")%2 != 0 {
		return nil
	}
	var out []Completion
	seen := map[string]bool{}
	for _, r := range recipients {
		for _, c := range []Completion{{r.Name, r.Name + " (" + r.Pane + ")"}, {r.Pane, r.Pane}, {r.Role, "Role: " + r.Role}} {
			if c.Token != "" && !seen[c.Token] && strings.HasPrefix(c.Token, loc[1]) {
				out = append(out, c)
				seen[c.Token] = true
			}
		}
	}
	return out[:min(8, len(out))]
}
func CompleteMention(input, token string) string {
	loc := addressSuffix.FindStringSubmatchIndex(input)
	if loc == nil {
		return input
	}
	return input[:loc[2]-1] + "@" + token + " "
}

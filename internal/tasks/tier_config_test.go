package tasks

import "testing"

func TestTierRoleParsing(t *testing.T) {
	ok := []string{
		"roles: {dev: {tier: fast, effort: medium}}",
		"roles: {dev: {agent: pi, model: m}}",
	}
	bad := []string{
		"roles: {dev: {tier: fast, agent: pi}}",
		"roles: {dev: {tier: fast, model: m}}",
		"roles: {dev: {tier: fast, skills: []}}",
		"roles: {dev: {tier: fast, effort: bogus}}",
		"roles: {dev: {effort: high}}",
	}
	for _, s := range ok {
		if _, err := ParseBoardConfig([]byte(s)); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range bad {
		if _, err := ParseBoardConfig([]byte(s)); err == nil {
			t.Errorf("want error: %s", s)
		}
	}
}

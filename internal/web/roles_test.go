package web

import "testing"

func TestContextSentOnlyWhenKeysPresent(t *testing.T) {
	if contextSent([]byte(`{"name":"a","model":"m"}`)) {
		t.Fatal("body without context keys reported as context")
	}
	if !contextSent([]byte(`{"name":"a","skills":null}`)) {
		t.Fatal("null skills is an explicit default and must count")
	}
	if !contextSent([]byte(`{"name":"a","tools":[]}`)) {
		t.Fatal("empty tools must count")
	}
}

package workflow

import "testing"

func TestChain(t *testing.T) {
	w, e := Parse([]byte("jobs:\n  a: {agent: reviewer}\n  b: {needs: a, run: echo ok, on-failure: a}\n"))
	if e != nil || len(w.Jobs) != 2 {
		t.Fatal(w, e)
	}
	for _, s := range []string{"jobs:\n  a: {run: x}\n  b: {run: y}\n", "jobs:\n  a: {run: x, needs: z}\n", "jobs:\n  a: {run: x, needs: b}\n  b: {run: y, needs: a}\n", "jobs:\n  a: {run: x, status: done}\n"} {
		if _, e := Parse([]byte(s)); e == nil {
			t.Fatal(s)
		}
	}
}
func TestExpand(t *testing.T) {
	got := Expand("${{ inputs.x }} ${{ jobs.a.outputs.k }} ${{ ticket.id }} ${{ unknown }}", map[string]string{"x": "${{ ticket.id }}"}, map[string]map[string]string{"a": {"k": "yes"}}, "id", "title")
	if got != "${{ ticket.id }} yes id " {
		t.Fatal(got)
	}
}

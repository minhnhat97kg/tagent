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

func TestErrorLine(t *testing.T) {
	for src, want := range map[string]int{
		"jobs:\n  a: {run: x}\n  a: {run: y}\n":                     3, // duplicate key is a yaml error, line from parser
		"jobs:\n  a: {run: x}\n  b: {run: y, needs: z}\n":           3,
		"jobs:\n  a: {run: x}\n  b: {run: y, needs: b}\n":           3,
		"jobs:\n  a:\n    run: x\n    bogus: 1\n":                   4,
		"jobs:\n  a: {run: x, needs: b}\n  b: {run: y, needs: a}\n": 2,
		"jobs:\n  a: {run: x}\n  b: {run: y, status: done}\n":       3,
		"jobs:\n  a: {agent: reviewer, run: x}\n":                   2,
	} {
		_, err := Parse([]byte(src))
		if err == nil {
			t.Fatalf("accepted %q", src)
		}
		if got := ErrorLine(err); got != want {
			t.Errorf("%q: line %d, want %d (%v)", src, got, want, err)
		}
	}
	if ErrorLine(nil) != 0 {
		t.Fatal("nil error has a line")
	}
}

func TestCleanupKey(t *testing.T) {
	const job = "jobs:\n  a: {run: x}\n"
	for src, want := range map[string]string{job: "", "cleanup: remove\n" + job: "remove", "cleanup: keep\n" + job: "keep"} {
		if w, err := Parse([]byte(src)); err != nil || w.Cleanup != want {
			t.Fatalf("%q: %q %v", src, w.Cleanup, err)
		}
	}
	for _, src := range []string{"cleanup: delete\n" + job, "cleanup:\n" + job} {
		if _, err := Parse([]byte(src)); ErrorLine(err) != 1 {
			t.Fatalf("%q accepted or wrong line: %v", src, err)
		}
	}
}

func TestWorkplaceKey(t *testing.T) {
	const job = "jobs:\n  a: {run: x}\n"
	for src, want := range map[string]string{job: "", "workplace: git-worktree\n" + job: "git-worktree", "workplace: none\n" + job: "none", "workplace: folder\n" + job: "folder", "workplace: browser\n" + job: "browser"} {
		if w, err := Parse([]byte(src)); err != nil || w.Workplace != want {
			t.Fatalf("%q: %q %v", src, w.Workplace, err)
		}
	}
	for _, src := range []string{"workplace: remote\n" + job, "workplace:\n" + job, "workplace: [none]\n" + job} {
		if _, err := Parse([]byte(src)); ErrorLine(err) != 1 {
			t.Fatalf("%q accepted or wrong line: %v", src, err)
		}
	}
}

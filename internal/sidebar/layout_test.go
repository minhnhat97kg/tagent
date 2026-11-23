package sidebar

import (
	"slices"
	"testing"
)

func TestSplitAlwaysSumsToTotal(t *testing.T) {
	sets := [][]constraint{
		{Percent(28).AtLeast(8), Fixed(10).AtLeast(8), Fill(1).AtLeast(6), Fixed(8)},
		{Fill(1).AtLeast(8), Fixed(8)},
		{Fill(1), Fill(3), Min(2), Max(5)},
		{Fixed(3), Fixed(4)},
		{Percent(33), Percent(33), Percent(34)},
	}
	for _, cs := range sets {
		for total := 0; total <= 80; total++ {
			got := split(total, cs...)
			sum := 0
			for _, v := range got {
				sum += v
			}
			if sum != total {
				t.Fatalf("split(%d, %d parts) = %v sums to %d", total, len(cs), got, sum)
			}
		}
	}
}

func TestSplit(t *testing.T) {
	dash := []constraint{Percent(28).AtLeast(8), Fixed(10).AtLeast(8), Fill(1).AtLeast(6), Fixed(8)}
	for _, c := range []struct {
		name  string
		total int
		cs    []constraint
		want  []int
	}{
		{"roomy: percent, fixed, fill gets the rest", 60, dash, []int{16, 10, 26, 8}},
		{"percent rounds down", 19, []constraint{Percent(28), Fill(1)}, []int{5, 14}},
		{"floors hold when they fit", 30, dash, []int{8, 8, 6, 8}},
		{"earlier parts grow first", 34, dash, []int{9, 10, 7, 8}},
		{"tight: tallest shrinks first", 20, dash, []int{5, 5, 5, 5}},
		{"focused card shrinks last", 20, []constraint{dash[0], dash[1], dash[2].Last(true), dash[3]}, []int{4, 5, 6, 5}},
		{"fill weights 1:3 with remainder to the earliest", 10, []constraint{Fill(1), Fill(3)}, []int{3, 7}},
		{"max caps", 20, []constraint{Max(4), Fill(1)}, []int{4, 16}},
		{"fixed only: fill the column anyway", 10, []constraint{Fixed(3), Fixed(4)}, []int{5, 5}},
		{"too short for everything", 3, dash, []int{0, 1, 1, 1}},
	} {
		if got := split(c.total, c.cs...); !slices.Equal(got, c.want) {
			t.Errorf("%s: split(%d) = %v, want %v", c.name, c.total, got, c.want)
		}
	}
}

func TestSplitRects(t *testing.T) {
	v := vsplit(rect{2, 1, 10, 20}, Fixed(5), Fill(1))
	if v[0] != (rect{2, 1, 10, 5}) || v[1] != (rect{2, 6, 10, 15}) {
		t.Fatalf("vsplit = %v", v)
	}
	h := hsplit(rect{0, 1, 90, 20}, Fixed(40), Fill(1))
	if h[0] != (rect{0, 1, 40, 20}) || h[1] != (rect{40, 1, 50, 20}) {
		t.Fatalf("hsplit = %v", h)
	}
}

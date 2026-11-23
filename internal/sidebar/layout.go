package sidebar

// Constraint layout (ratatui style): split cuts a length into parts that
// always add up to the total. Every part starts at its floor; if even the
// floors do not fit, the tallest parts lose rows one at a time (parts marked
// Last() only after the others). Otherwise parts grow in order toward what
// they want (Fixed n, Percent p, Max n) and the spare goes to the flexible
// parts (Fill by weight, Min with weight 1), else round-robin to all.
type constraint struct {
	kind  int // 0 fixed, 1 percent, 2 min, 3 max, 4 fill
	n     int // rows, percent or fill weight
	floor int // smallest size while there is room (default: see floorOf)
	last  bool
}

const (
	cFixed = iota
	cPercent
	cMin
	cMax
	cFill
)

func Fixed(n int) constraint   { return constraint{kind: cFixed, n: n} }
func Percent(p int) constraint { return constraint{kind: cPercent, n: p} }
func Min(n int) constraint     { return constraint{kind: cMin, n: n} }
func Max(n int) constraint     { return constraint{kind: cMax, n: n} }
func Fill(weight int) constraint {
	return constraint{kind: cFill, n: max(1, weight)}
}

// AtLeast sets the smallest size the part keeps while there is room.
func (c constraint) AtLeast(n int) constraint { c.floor = n; return c }

// Last makes the part the final one to lose rows when the total is too small.
func (c constraint) Last(on bool) constraint { c.last = on; return c }

func (c constraint) floorOf() int {
	switch {
	case c.floor > 0:
		return c.floor
	case c.kind == cFixed || c.kind == cMin:
		return c.n
	}
	return 0
}

func split(total int, cs ...constraint) []int {
	out := make([]int, len(cs))
	if total <= 0 || len(cs) == 0 {
		return out
	}
	floorSum := 0
	for i, c := range cs {
		out[i] = max(0, c.floorOf())
		floorSum += out[i]
	}
	if floorSum > total {
		for stage := 0; stage < 4 && floorSum > total; stage++ { // not-last parts first, then Last(); down to one row, then to none
			for lo := 1 - stage/2; floorSum > total; {
				pick := -1
				for i, c := range cs {
					if c.last == (stage%2 == 1) && out[i] > lo && (pick < 0 || out[i] > out[pick]) {
						pick = i
					}
				}
				if pick < 0 {
					break
				}
				out[pick]--
				floorSum--
			}
		}
		return out
	}
	left := total - floorSum
	for i, c := range cs {
		want := c.n
		switch c.kind {
		case cPercent:
			want = total * c.n / 100
		case cMin, cFill:
			continue
		}
		grow := min(left, max(0, want-out[i]))
		out[i] += grow
		left -= grow
	}
	var slots []int // one slot per unit of weight, so remainders land on the earliest parts
	for i, c := range cs {
		switch c.kind {
		case cFill:
			for k := 0; k < c.n; k++ {
				slots = append(slots, i)
			}
		case cMin:
			slots = append(slots, i)
		}
	}
	if len(slots) == 0 {
		for i := range cs {
			slots = append(slots, i)
		}
	}
	// ponytail: whole weights share evenly, the rest one row at a time in order
	each, rest := left/len(slots), left%len(slots)
	for k, i := range slots {
		out[i] += each + b2i(k < rest)
	}
	return out
}

// vsplit stacks rows inside r; hsplit puts columns side by side.
func vsplit(r rect, cs ...constraint) []rect {
	rs, y := make([]rect, len(cs)), r.y
	for i, h := range split(r.h, cs...) {
		rs[i] = rect{r.x, y, r.w, h}
		y += h
	}
	return rs
}

func hsplit(r rect, cs ...constraint) []rect {
	rs, x := make([]rect, len(cs)), r.x
	for i, w := range split(r.w, cs...) {
		rs[i] = rect{x, r.y, w, r.h}
		x += w
	}
	return rs
}

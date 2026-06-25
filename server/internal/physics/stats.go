package physics

// Stats holds FIFA-scale attributes as integers in [0, 99]. Construct through
// Clamped (or AddPlayer, which clamps) to guarantee the range. Only Pace and
// Dribbling are wired into physics this phase; the rest are defined but unused.
type Stats struct {
	Pace      int
	Shooting  int
	Passing   int
	Dribbling int
	Defending int
	Physical  int
}

const (
	StatMin = 0
	StatMax = 99
)

func clampStat(v int) int {
	if v < StatMin {
		return StatMin
	}
	if v > StatMax {
		return StatMax
	}
	return v
}

// Clamped returns a copy with every attribute constrained to [0, 99].
func (s Stats) Clamped() Stats {
	return Stats{
		Pace:      clampStat(s.Pace),
		Shooting:  clampStat(s.Shooting),
		Passing:   clampStat(s.Passing),
		Dribbling: clampStat(s.Dribbling),
		Defending: clampStat(s.Defending),
		Physical:  clampStat(s.Physical),
	}
}

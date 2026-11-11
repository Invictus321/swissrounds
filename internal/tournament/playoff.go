package tournament

import (
	"errors"
	"fmt"
	"math/bits"
	"sort"
)

// Playoff is a single-elimination top cut seeded from the Swiss standings.
type Playoff struct {
	Size   int      `json:"size"`
	Seeds  []int    `json:"seeds"` // player IDs; index 0 is seed 1
	Rounds []*Round `json:"rounds"`
}

// BracketOrder returns 1-based seeds in bracket position order so that the
// top seeds can only meet in later rounds, e.g. 8 -> [1 8 4 5 2 7 3 6].
func BracketOrder(n int) []int {
	order := []int{1}
	for size := 2; size <= n; size *= 2 {
		next := make([]int, 0, size)
		for _, s := range order {
			next = append(next, s, size+1-s)
		}
		order = next
	}
	return order
}

// StartPlayoff cuts to the top N active players of the Swiss standings.
func (t *Tournament) StartPlayoff() error {
	if t.Playoff != nil {
		return errors.New("the top cut has already started")
	}
	if t.TopCut == 0 {
		return errors.New("this tournament has no top cut")
	}
	cur := t.CurrentRound()
	if cur == nil || !cur.Complete() {
		return errors.New("finish reporting the current Swiss round first")
	}
	var seeds []int
	for _, s := range t.Standings() {
		if !s.Dropped && len(seeds) < t.TopCut {
			seeds = append(seeds, s.PlayerID)
		}
	}
	if len(seeds) < t.TopCut {
		return fmt.Errorf("only %d active players; need %d for the top cut", len(seeds), t.TopCut)
	}
	p := &Playoff{Size: t.TopCut, Seeds: seeds}
	order := BracketOrder(t.TopCut)
	r := &Round{Number: 1}
	for i := 0; i < len(order); i += 2 {
		r.Matches = append(r.Matches, &Match{Table: i/2 + 1, P1: seeds[order[i]-1], P2: seeds[order[i+1]-1]})
	}
	p.Rounds = []*Round{r}
	t.Playoff = p
	t.Timer.Reset()
	return nil
}

// CancelPlayoff removes the top cut so Swiss rounds can be edited again.
func (t *Tournament) CancelPlayoff() error {
	if t.Playoff == nil {
		return errors.New("the top cut has not started")
	}
	t.Playoff = nil
	t.Timer.Reset()
	return nil
}

func (p *Playoff) numRounds() int { return bits.Len(uint(p.Size)) - 1 }

func (p *Playoff) seed(id int) int {
	for i, s := range p.Seeds {
		if s == id {
			return i + 1
		}
	}
	return 0
}

// RoundName returns e.g. "Quarterfinals" for a 1-based playoff round.
func (p *Playoff) RoundName(round int) string {
	switch p.numRounds() - round {
	case 0:
		return "Finals"
	case 1:
		return "Semifinals"
	case 2:
		return "Quarterfinals"
	}
	return fmt.Sprintf("Top %d", p.Size>>(round-1))
}

// Champion returns the winner of the final, or 0 if undecided.
func (p *Playoff) Champion() int {
	if len(p.Rounds) != p.numRounds() {
		return 0
	}
	return p.Rounds[len(p.Rounds)-1].Matches[0].Winner()
}

// advance creates or updates later playoff rounds from reported winners.
// It returns true if a new round was created.
func (p *Playoff) advance() bool {
	for i := 0; i < len(p.Rounds); i++ {
		r := p.Rounds[i]
		if len(r.Matches) == 1 || !r.Complete() {
			p.Rounds = p.Rounds[:i+1]
			return false
		}
		next := &Round{Number: i + 2}
		for j := 0; j < len(r.Matches); j += 2 {
			a, b := r.Matches[j].Winner(), r.Matches[j+1].Winner()
			if p.seed(b) < p.seed(a) {
				a, b = b, a
			}
			next.Matches = append(next.Matches, &Match{Table: j/2 + 1, P1: a, P2: b})
		}
		if i+1 == len(p.Rounds) {
			p.Rounds = append(p.Rounds, next)
			return true
		}
		for j, m := range next.Matches {
			e := p.Rounds[i+1].Matches[j]
			if e.P1 != m.P1 || e.P2 != m.P2 {
				e.P1, e.P2 = m.P1, m.P2
				e.clearResult()
			}
		}
	}
	return false
}

// applyFinishes moves top cut players to the top of the standings ordered by
// how far they progressed (ties broken by seed) and labels their finish.
func (p *Playoff) applyFinishes(st []Standing) {
	type finish struct {
		level int
		label string
	}
	R := p.numRounds()
	fin := map[int]finish{}
	for _, id := range p.Seeds {
		fin[id] = finish{level: 0, label: fmt.Sprintf("Top %d", p.Size)}
	}
	for ri, r := range p.Rounds {
		for _, m := range r.Matches {
			for _, id := range []int{m.P1, m.P2} {
				label := fmt.Sprintf("Top %d", p.Size>>ri)
				level := 2*ri + 1 // still alive in this round
				if w := m.Winner(); w != 0 {
					if w != id {
						level = 2 * ri
						if ri == R-1 {
							label = "Finalist"
						}
					} else if ri == R-1 {
						level, label = 2*R, "Champion"
					}
				}
				fin[id] = finish{level, label}
			}
		}
	}
	for i := range st {
		if f, ok := fin[st[i].PlayerID]; ok {
			st[i].PlayoffSeed = p.seed(st[i].PlayerID)
			st[i].Finish = f.label
		}
	}
	sort.SliceStable(st, func(i, j int) bool {
		fi, iok := fin[st[i].PlayerID]
		fj, jok := fin[st[j].PlayerID]
		if iok != jok {
			return iok
		}
		if !iok {
			return false
		}
		if fi.level != fj.level {
			return fi.level > fj.level
		}
		return st[i].PlayoffSeed < st[j].PlayoffSeed
	})
}

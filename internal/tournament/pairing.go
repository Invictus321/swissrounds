package tournament

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
)

// pairingBudget caps the backtracking search before falling back to a
// pairing that allows rematches.
const pairingBudget = 500_000

// PairNextRound generates pairings for the next Swiss round.
//
// Players are grouped by match points (randomly ordered within a point
// group) and paired top-down, avoiding rematches and pairing down only when
// necessary. With an odd number of players, the lowest-ranked player who has
// not yet had a bye receives one (counted as a 2-0 match win).
func (t *Tournament) PairNextRound(rng *rand.Rand) (*Round, error) {
	if t.Playoff != nil {
		return nil, errors.New("the top cut has already started")
	}
	if r := t.CurrentRound(); r != nil && !r.Complete() {
		return nil, fmt.Errorf("round %d still has unreported results", r.Number)
	}
	if len(t.Rounds) >= t.NumRounds {
		return nil, fmt.Errorf("all %d Swiss rounds have been played (increase the round count in settings to add more)", t.NumRounds)
	}

	recs := t.records(len(t.Rounds))
	played := map[int]map[int]bool{}
	for _, r := range t.Rounds {
		for _, m := range r.Matches {
			if m.IsBye() {
				continue
			}
			for _, pr := range [][2]int{{m.P1, m.P2}, {m.P2, m.P1}} {
				if played[pr[0]] == nil {
					played[pr[0]] = map[int]bool{}
				}
				played[pr[0]][pr[1]] = true
			}
		}
	}

	var active []int
	for _, p := range t.Players {
		if !p.Dropped {
			active = append(active, p.ID)
		}
	}
	if len(active) < 2 {
		return nil, errors.New("at least 2 active players are needed to pair a round")
	}
	rng.Shuffle(len(active), func(i, j int) { active[i], active[j] = active[j], active[i] })
	sort.SliceStable(active, func(i, j int) bool { return recs[active[i]].points > recs[active[j]].points })

	var pairs [][2]int
	bye := 0
	ok := false
	if len(active)%2 == 0 {
		budget := pairingBudget
		pairs, ok = findPairing(active, played, &budget)
	} else {
		// Bye candidates: lowest points first, players without a bye before
		// those who have already had one.
		cands := make([]int, len(active))
		for i, id := range active {
			cands[len(active)-1-i] = id
		}
		sort.SliceStable(cands, func(i, j int) bool {
			return recs[cands[i]].byes == 0 && recs[cands[j]].byes > 0
		})
		budget := pairingBudget
		for _, c := range cands {
			pairs, ok = findPairing(without(active, c), played, &budget)
			if ok {
				bye = c
				break
			}
			if budget <= 0 {
				break
			}
		}
		if !ok {
			bye = cands[0]
		}
	}
	if !ok {
		rest := active
		if bye != 0 {
			rest = without(active, bye)
		}
		pairs = greedyPairing(rest, played)
	}

	round := &Round{Number: len(t.Rounds) + 1}
	for i, pr := range pairs {
		round.Matches = append(round.Matches, &Match{Table: i + 1, P1: pr[0], P2: pr[1]})
	}
	if bye != 0 {
		m := &Match{P1: bye}
		m.setBye()
		round.Matches = append(round.Matches, m)
	}
	t.Rounds = append(t.Rounds, round)
	t.Timer.Reset()
	return round, nil
}

func without(ids []int, x int) []int {
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id != x {
			out = append(out, id)
		}
	}
	return out
}

// findPairing pairs players (sorted best-first) with backtracking so that no
// two players who have already played are paired again.
func findPairing(players []int, played map[int]map[int]bool, budget *int) ([][2]int, bool) {
	if len(players) == 0 {
		return nil, true
	}
	if *budget <= 0 {
		return nil, false
	}
	*budget--
	p := players[0]
	for i := 1; i < len(players); i++ {
		q := players[i]
		if played[p][q] {
			continue
		}
		rest := make([]int, 0, len(players)-2)
		rest = append(rest, players[1:i]...)
		rest = append(rest, players[i+1:]...)
		if sub, ok := findPairing(rest, played, budget); ok {
			return append([][2]int{{p, q}}, sub...), true
		}
		if *budget <= 0 {
			return nil, false
		}
	}
	return nil, false
}

// greedyPairing is a fallback that prefers new opponents but allows rematches.
func greedyPairing(players []int, played map[int]map[int]bool) [][2]int {
	rest := append([]int(nil), players...)
	var pairs [][2]int
	for len(rest) >= 2 {
		p := rest[0]
		pick := 1
		for i := 1; i < len(rest); i++ {
			if !played[p][rest[i]] {
				pick = i
				break
			}
		}
		pairs = append(pairs, [2]int{p, rest[pick]})
		rest = append(rest[1:pick], rest[pick+1:]...)
	}
	return pairs
}

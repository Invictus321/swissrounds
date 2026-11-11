package tournament

import (
	"math"
	"sort"
)

// MinPercentage is the floor applied to match/game win percentages (MTR Appendix C).
const MinPercentage = 1.0 / 3.0

// Standing is a player's Swiss record and tiebreakers.
type Standing struct {
	Rank        int     `json:"rank"`
	PlayerID    int     `json:"playerId"`
	Name        string  `json:"name"`
	Dropped     bool    `json:"dropped"`
	Points      int     `json:"points"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	Draws       int     `json:"draws"`
	Byes        int     `json:"byes"`
	GameWins    int     `json:"gameWins"`
	GameLosses  int     `json:"gameLosses"`
	GameDraws   int     `json:"gameDraws"`
	MWP         float64 `json:"mwp"`
	OMW         float64 `json:"omw"`
	GWP         float64 `json:"gwp"`
	OGW         float64 `json:"ogw"`
	PlayoffSeed int     `json:"playoffSeed,omitempty"`
	Finish      string  `json:"finish,omitempty"`
}

type record struct {
	points, wins, losses, draws, byes int
	gameWins, gameLosses, gameDraws   int
	matches                           int
	opponents                         []int
}

func (r *record) mwp() float64 {
	if r.matches == 0 {
		return 0
	}
	return math.Max(MinPercentage, float64(r.points)/float64(3*r.matches))
}

func (r *record) gwp() float64 {
	games := r.gameWins + r.gameLosses + r.gameDraws
	if games == 0 {
		if r.matches == 0 {
			return 0
		}
		return MinPercentage
	}
	return math.Max(MinPercentage, float64(3*r.gameWins+r.gameDraws)/float64(3*games))
}

// records tallies reported results from the first `through` Swiss rounds.
func (t *Tournament) records(through int) map[int]*record {
	recs := make(map[int]*record, len(t.Players))
	for _, p := range t.Players {
		recs[p.ID] = &record{}
	}
	for i, r := range t.Rounds {
		if i >= through {
			break
		}
		for _, m := range r.Matches {
			if !m.Reported {
				continue
			}
			a := recs[m.P1]
			if m.IsBye() {
				a.byes++
				a.matches++
				a.wins++
				a.points += 3
				a.gameWins += 2
				continue
			}
			b := recs[m.P2]
			a.matches++
			b.matches++
			a.opponents = append(a.opponents, m.P2)
			b.opponents = append(b.opponents, m.P1)
			a.gameWins += m.P1Wins
			a.gameLosses += m.P2Wins
			a.gameDraws += m.Draws
			b.gameWins += m.P2Wins
			b.gameLosses += m.P1Wins
			b.gameDraws += m.Draws
			switch {
			case m.P1Wins > m.P2Wins:
				a.wins++
				a.points += 3
				b.losses++
			case m.P2Wins > m.P1Wins:
				b.wins++
				b.points += 3
				a.losses++
			default:
				a.draws++
				b.draws++
				a.points++
				b.points++
			}
		}
	}
	return recs
}

// Standings returns standings including every reported Swiss result.
func (t *Tournament) Standings() []Standing {
	return t.StandingsAfter(len(t.Rounds))
}

// CompletedRounds is the number of Swiss rounds with every result reported.
func (t *Tournament) CompletedRounds() int {
	n := len(t.Rounds)
	if r := t.CurrentRound(); r != nil && !r.Complete() {
		n--
	}
	return n
}

// StandingsAfter returns standings using the first `rounds` Swiss rounds,
// ranked by match points, opponents' match-win %, game-win %, opponents'
// game-win %, then a random tiebreaker. Once a top cut has started, the top
// players are ordered by how far they progressed in the playoff.
func (t *Tournament) StandingsAfter(rounds int) []Standing {
	recs := t.records(rounds)
	out := make([]Standing, 0, len(t.Players))
	tiebreak := make(map[int]int64, len(t.Players))
	for _, p := range t.Players {
		r := recs[p.ID]
		s := Standing{
			PlayerID: p.ID, Name: p.Name, Dropped: p.Dropped,
			Points: r.points, Wins: r.wins, Losses: r.losses, Draws: r.draws, Byes: r.byes,
			GameWins: r.gameWins, GameLosses: r.gameLosses, GameDraws: r.gameDraws,
			MWP: r.mwp(), GWP: r.gwp(),
		}
		if len(r.opponents) > 0 {
			for _, o := range r.opponents {
				s.OMW += recs[o].mwp()
				s.OGW += recs[o].gwp()
			}
			s.OMW /= float64(len(r.opponents))
			s.OGW /= float64(len(r.opponents))
		}
		tiebreak[p.ID] = p.Tiebreak
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if c := cmpFloat(a.OMW, b.OMW); c != 0 {
			return c > 0
		}
		if c := cmpFloat(a.GWP, b.GWP); c != 0 {
			return c > 0
		}
		if c := cmpFloat(a.OGW, b.OGW); c != 0 {
			return c > 0
		}
		return tiebreak[a.PlayerID] < tiebreak[b.PlayerID]
	})
	if t.Playoff != nil {
		t.Playoff.applyFinishes(out)
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func cmpFloat(a, b float64) int {
	const eps = 1e-9
	switch {
	case a > b+eps:
		return 1
	case b > a+eps:
		return -1
	}
	return 0
}

// PointsBefore returns each player's match points from rounds before the
// given 1-based round number (i.e. their points going into that round).
func (t *Tournament) PointsBefore(round int) map[int]int {
	recs := t.records(round - 1)
	out := make(map[int]int, len(recs))
	for id, r := range recs {
		out[id] = r.points
	}
	return out
}

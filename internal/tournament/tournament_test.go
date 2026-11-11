package tournament

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

func names(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("Player %d", i+1)
	}
	return out
}

func newT(t *testing.T, n, topCut int, seed uint64) (*Tournament, *rand.Rand) {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, seed))
	tr, err := New(Config{Players: names(n), TopCut: topCut}, rng)
	if err != nil {
		t.Fatal(err)
	}
	return tr, rng
}

func reportRandom(t *testing.T, tr *Tournament, rng *rand.Rand) {
	t.Helper()
	r := tr.CurrentRound()
	results := [][3]int{{2, 0, 0}, {2, 1, 0}, {0, 2, 0}, {1, 2, 0}, {1, 1, 1}}
	for i, m := range r.Matches {
		if m.IsBye() {
			continue
		}
		res := results[rng.IntN(len(results))]
		if err := tr.ReportResult(StageSwiss, r.Number, i, res[0], res[1], res[2]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSwissPairingsAreValid(t *testing.T) {
	for _, n := range []int{5, 8, 9, 16, 23, 40} {
		for seed := uint64(1); seed <= 20; seed++ {
			tr, rng := newT(t, n, 0, seed)
			played := map[[2]int]bool{}
			byes := map[int]bool{}
			for range tr.NumRounds {
				r, err := tr.PairNextRound(rng)
				if err != nil {
					t.Fatal(err)
				}
				seen := map[int]bool{}
				for _, m := range r.Matches {
					for _, id := range []int{m.P1, m.P2} {
						if id == 0 {
							continue
						}
						if seen[id] {
							t.Fatalf("n=%d seed=%d: player %d paired twice in round %d", n, seed, id, r.Number)
						}
						seen[id] = true
					}
					if m.IsBye() {
						if byes[m.P1] {
							t.Fatalf("n=%d seed=%d: player %d got a second bye", n, seed, m.P1)
						}
						byes[m.P1] = true
						continue
					}
					key := [2]int{min(m.P1, m.P2), max(m.P1, m.P2)}
					if played[key] {
						t.Fatalf("n=%d seed=%d: rematch %v in round %d", n, seed, key, r.Number)
					}
					played[key] = true
				}
				if len(seen) != n {
					t.Fatalf("n=%d: %d players paired, want %d", n, len(seen), n)
				}
				reportRandom(t, tr, rng)
			}
			if tr.Phase() != PhaseComplete {
				t.Fatalf("phase = %s, want complete", tr.Phase())
			}
		}
	}
}

func TestPairingGroupsByPoints(t *testing.T) {
	tr, rng := newT(t, 8, 0, 7)
	tr.PairNextRound(rng)
	// P1 wins every round-1 match.
	for i := range tr.CurrentRound().Matches {
		tr.ReportResult(StageSwiss, 1, i, 2, 0, 0)
	}
	pts := tr.PointsBefore(2)
	r, _ := tr.PairNextRound(rng)
	for _, m := range r.Matches {
		if pts[m.P1] != pts[m.P2] {
			t.Fatalf("round 2 paired %d points vs %d points", pts[m.P1], pts[m.P2])
		}
	}
}

func TestDroppedPlayersAreNotPaired(t *testing.T) {
	tr, rng := newT(t, 6, 0, 3)
	tr.PairNextRound(rng)
	reportRandom(t, tr, rng)
	tr.SetDropped(2, true)
	r, err := tr.PairNextRound(rng)
	if err != nil {
		t.Fatal(err)
	}
	byes := 0
	for _, m := range r.Matches {
		if m.P1 == 2 || m.P2 == 2 {
			t.Fatal("dropped player was paired")
		}
		if m.IsBye() {
			byes++
		}
	}
	if byes != 1 {
		t.Fatalf("got %d byes for 5 active players, want 1", byes)
	}
}

func TestCannotPairWithUnreportedResults(t *testing.T) {
	tr, rng := newT(t, 4, 0, 1)
	tr.PairNextRound(rng)
	if _, err := tr.PairNextRound(rng); err == nil {
		t.Fatal("expected error pairing with outstanding results")
	}
}

func TestTiebreakers(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	tr, _ := New(Config{Players: []string{"A", "B", "C", "D"}, NumRounds: 2}, rng)
	// Round 1: A beats B 2-1, C draws D 1-1-1.
	// Round 2: A beats C 2-0, B beats D 2-0.
	tr.Rounds = []*Round{
		{Number: 1, Matches: []*Match{{Table: 1, P1: 1, P2: 2, P1Wins: 2, P2Wins: 1, Reported: true}, {Table: 2, P1: 3, P2: 4, P1Wins: 1, P2Wins: 1, Draws: 1, Reported: true}}},
		{Number: 2, Matches: []*Match{{Table: 1, P1: 1, P2: 3, P1Wins: 2, Reported: true}, {Table: 2, P1: 2, P2: 4, P1Wins: 2, Reported: true}}},
	}
	st := tr.Standings()
	by := map[string]Standing{}
	for _, s := range st {
		by[s.Name] = s
	}
	approx := func(name, what string, got, want float64) {
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%s %s = %.4f, want %.4f", name, what, got, want)
		}
	}
	// MWPs: A 1.0, B 0.5, C 1/6->1/3, D 1/6->1/3.
	approx("A", "OMW", by["A"].OMW, (0.5+1.0/3)/2)
	approx("B", "OMW", by["B"].OMW, (1.0+1.0/3)/2)
	approx("A", "GWP", by["A"].GWP, 4.0/5)
	approx("C", "GWP", by["C"].GWP, 1.0/3) // (3+1)/15 floored
	// B GWP = 3/(3*5)... games W3 L2 -> 9/15 = 0.6; D: W1 L3 D1 -> 4/15 -> floored.
	approx("A", "OGW", by["A"].OGW, (0.6+1.0/3)/2)
	if by["A"].Points != 6 || by["B"].Points != 3 || by["C"].Points != 1 || by["D"].Points != 1 {
		t.Errorf("unexpected points: %+v", by)
	}
	if st[0].Name != "A" || st[1].Name != "B" {
		t.Errorf("unexpected order: %s, %s", st[0].Name, st[1].Name)
	}
	// C and D are tied on points; C's OMW (A,D) beats D's OMW (C,B).
	if st[2].Name != "C" {
		t.Errorf("3rd = %s, want C", st[2].Name)
	}
}

func TestByeCountsAsTwoZeroWin(t *testing.T) {
	tr, rng := newT(t, 3, 0, 5)
	r, _ := tr.PairNextRound(rng)
	var bye int
	for _, m := range r.Matches {
		if m.IsBye() {
			bye = m.P1
		}
	}
	for _, s := range tr.Standings() {
		if s.PlayerID == bye {
			if s.Points != 3 || s.GameWins != 2 || s.Byes != 1 || s.OMW != 0 {
				t.Fatalf("bye standing = %+v", s)
			}
		}
	}
}

func TestBracketOrder(t *testing.T) {
	if got := BracketOrder(8); !slices.Equal(got, []int{1, 8, 4, 5, 2, 7, 3, 6}) {
		t.Fatalf("BracketOrder(8) = %v", got)
	}
}

func TestTopEightPlayoff(t *testing.T) {
	tr, rng := newT(t, 12, 8, 11)
	for range tr.NumRounds {
		if _, err := tr.PairNextRound(rng); err != nil {
			t.Fatal(err)
		}
		reportRandom(t, tr, rng)
	}
	// Drop the current 3rd place player; they must not make the cut.
	dropped := tr.Standings()[2].PlayerID
	tr.SetDropped(dropped, true)
	if tr.Phase() != PhaseSwissDone {
		t.Fatalf("phase = %s", tr.Phase())
	}
	if err := tr.StartPlayoff(); err != nil {
		t.Fatal(err)
	}
	p := tr.Playoff
	if slices.Contains(p.Seeds, dropped) {
		t.Fatal("dropped player made top 8")
	}
	qf := p.Rounds[0].Matches
	if qf[0].P1 != p.Seeds[0] || qf[0].P2 != p.Seeds[7] || qf[1].P1 != p.Seeds[3] || qf[1].P2 != p.Seeds[4] {
		t.Fatal("quarterfinals not seeded 1v8, 4v5")
	}
	if err := tr.ReportResult(StagePlayoff, 1, 0, 1, 1, 1); err == nil {
		t.Fatal("expected error for drawn top 8 match")
	}
	// Lower seeds win every QF; higher seed wins every later match.
	for i := range qf {
		tr.ReportResult(StagePlayoff, 1, i, 0, 2, 0)
	}
	if len(p.Rounds) != 2 || p.RoundName(2) != "Semifinals" {
		t.Fatalf("semifinals not created")
	}
	sf := p.Rounds[1].Matches
	if sf[0].P1 != p.Seeds[4] || sf[0].P2 != p.Seeds[7] {
		t.Fatalf("SF1 = %d v %d, want seeds 5 v 8", p.seed(sf[0].P1), p.seed(sf[0].P2))
	}
	// Changing a QF winner after the SF is reported must be refused.
	tr.ReportResult(StagePlayoff, 2, 0, 2, 0, 0)
	if err := tr.ReportResult(StagePlayoff, 1, 0, 2, 0, 0); err == nil {
		t.Fatal("expected error changing QF winner after SF reported")
	}
	tr.ReportResult(StagePlayoff, 2, 1, 2, 1, 0)
	tr.ReportResult(StagePlayoff, 3, 0, 2, 0, 0)
	if p.Champion() != p.Seeds[4] || tr.Phase() != PhaseComplete {
		t.Fatalf("champion = seed %d, phase %s", p.seed(p.Champion()), tr.Phase())
	}
	st := tr.Standings()
	if st[0].Finish != "Champion" || st[1].Finish != "Finalist" || st[2].Finish != "Top 4" || st[4].Finish != "Top 8" || st[8].Finish != "" {
		t.Fatalf("unexpected finishes: %v %v %v %v %v", st[0].Finish, st[1].Finish, st[2].Finish, st[4].Finish, st[8].Finish)
	}
	// Clearing the final then an SF works in reverse order only.
	if err := tr.ClearResult(StagePlayoff, 2, 0); err == nil {
		t.Fatal("expected error clearing SF while final is reported")
	}
	if err := tr.ClearResult(StagePlayoff, 3, 0); err != nil {
		t.Fatal(err)
	}
	if err := tr.ClearResult(StagePlayoff, 2, 0); err != nil || len(p.Rounds) != 2 {
		t.Fatalf("clear SF: err=%v rounds=%d", err, len(p.Rounds))
	}
}

func TestSwapPlayers(t *testing.T) {
	tr, rng := newT(t, 4, 0, 2)
	r, _ := tr.PairNextRound(rng)
	a, b := r.Matches[0].P1, r.Matches[1].P1
	if err := tr.SwapPlayers(a, b); err != nil {
		t.Fatal(err)
	}
	if r.Matches[0].P1 != b || r.Matches[1].P1 != a {
		t.Fatal("players not swapped")
	}
	if err := tr.SwapPlayers(r.Matches[0].P1, r.Matches[0].P2); err == nil {
		t.Fatal("expected error swapping opponents")
	}
}

func TestTimer(t *testing.T) {
	var tm Timer
	tm.SetDuration(50 * 60_000)
	now := time.Unix(1000, 0)
	tm.Start(now)
	now = now.Add(10 * time.Minute)
	if got := tm.Remaining(now); got != 40*60_000 {
		t.Fatalf("remaining = %d", got)
	}
	tm.Pause(now)
	tm.Adjust(5 * 60_000)
	if got := tm.Remaining(now.Add(time.Hour)); got != 45*60_000 {
		t.Fatalf("remaining after pause+adjust = %d", got)
	}
}

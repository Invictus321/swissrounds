// Package tournament implements Magic: The Gathering style Swiss tournaments
// with single-elimination top cuts.
package tournament

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// Stages for result reporting.
const (
	StageSwiss   = "swiss"
	StagePlayoff = "playoff"
)

// Phases a tournament moves through.
const (
	PhaseRegistration = "registration"
	PhaseSwiss        = "swiss"
	PhaseSwissDone    = "swiss-complete"
	PhasePlayoff      = "playoff"
	PhaseComplete     = "complete"
)

// DisplayModes are the views the big screen can show.
var DisplayModes = []string{"pairings", "standings", "bracket", "timer"}

const (
	maxGameWins  = 3
	maxGameDraws = 5
)

type Player struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Dropped bool   `json:"dropped"`
	// Tiebreak is a random value used as the final (deterministic) tiebreaker.
	Tiebreak int64 `json:"tiebreak"`
}

type Match struct {
	Table    int  `json:"table"`
	P1       int  `json:"p1"`
	P2       int  `json:"p2"` // 0 means P1 has a bye
	P1Wins   int  `json:"p1Wins"`
	P2Wins   int  `json:"p2Wins"`
	Draws    int  `json:"draws"`
	Reported bool `json:"reported"`
}

func (m *Match) IsBye() bool { return m.P2 == 0 }

// Winner returns the winning player ID, or 0 for a draw or unreported match.
func (m *Match) Winner() int {
	switch {
	case !m.Reported:
		return 0
	case m.IsBye():
		return m.P1
	case m.P1Wins > m.P2Wins:
		return m.P1
	case m.P2Wins > m.P1Wins:
		return m.P2
	}
	return 0
}

func (m *Match) clearResult() {
	m.P1Wins, m.P2Wins, m.Draws, m.Reported = 0, 0, 0, false
}

func (m *Match) setBye() {
	m.Table, m.P1Wins, m.P2Wins, m.Draws, m.Reported = 0, 2, 0, 0, true
}

type Round struct {
	Number  int      `json:"number"`
	Matches []*Match `json:"matches"`
}

func (r *Round) Complete() bool {
	return r.Reported() == len(r.Matches)
}

func (r *Round) Reported() int {
	n := 0
	for _, m := range r.Matches {
		if m.Reported {
			n++
		}
	}
	return n
}

type Tournament struct {
	Name         string    `json:"name"`
	Players      []*Player `json:"players"`
	NumRounds    int       `json:"numRounds"`
	TopCut       int       `json:"topCut"`
	Rounds       []*Round  `json:"rounds"`
	Playoff      *Playoff  `json:"playoff,omitempty"`
	Timer        Timer     `json:"timer"`
	Display      string    `json:"display"`
	Announcement string    `json:"announcement"`
	NextPlayerID int       `json:"nextPlayerId"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Config holds the options for creating a tournament.
type Config struct {
	Name         string
	Players      []string
	NumRounds    int // 0 = recommended for the player count
	TopCut       int // 0 = no cut
	RoundMinutes int // 0 = 50
}

// RecommendedRounds returns the suggested number of Swiss rounds for a
// player count (following Wizards' recommendations for events with a cut).
func RecommendedRounds(players int) int {
	switch {
	case players <= 1:
		return 0
	case players <= 2:
		return 1
	case players <= 4:
		return 2
	case players <= 8:
		return 3
	case players <= 16:
		return 4
	case players <= 32:
		return 5
	case players <= 64:
		return 6
	case players <= 128:
		return 7
	case players <= 226:
		return 8
	case players <= 409:
		return 9
	}
	return 10
}

func validTopCut(n int) bool {
	return n == 0 || n == 2 || n == 4 || n == 8 || n == 16
}

func New(cfg Config, rng *rand.Rand) (*Tournament, error) {
	t := &Tournament{
		Name:         strings.TrimSpace(cfg.Name),
		NextPlayerID: 1,
		Rounds:       []*Round{},
		Display:      "pairings",
		CreatedAt:    time.Now(),
	}
	if t.Name == "" {
		t.Name = "Magic: The Gathering Tournament"
	}
	for _, n := range cfg.Players {
		if strings.TrimSpace(n) == "" {
			continue
		}
		if err := t.AddPlayer(n, rng); err != nil {
			return nil, err
		}
	}
	if len(t.Players) < 2 {
		return nil, errors.New("at least 2 players are required")
	}
	t.NumRounds = cfg.NumRounds
	if t.NumRounds <= 0 {
		t.NumRounds = RecommendedRounds(len(t.Players))
	}
	if !validTopCut(cfg.TopCut) {
		return nil, fmt.Errorf("top cut must be 0, 2, 4, 8 or 16 (got %d)", cfg.TopCut)
	}
	if cfg.TopCut > len(t.Players) {
		return nil, fmt.Errorf("a top %d needs at least %d players", cfg.TopCut, cfg.TopCut)
	}
	t.TopCut = cfg.TopCut
	mins := cfg.RoundMinutes
	if mins <= 0 {
		mins = 50
	}
	t.Timer.SetDuration(int64(mins) * 60_000)
	return t, nil
}

func (t *Tournament) Player(id int) *Player {
	for _, p := range t.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (t *Tournament) AddPlayer(name string, rng *rand.Rand) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("player name is required")
	}
	if t.Playoff != nil {
		return errors.New("cannot add players once the top cut has started")
	}
	for _, p := range t.Players {
		if strings.EqualFold(p.Name, name) {
			return fmt.Errorf("duplicate player name %q", name)
		}
	}
	t.Players = append(t.Players, &Player{ID: t.NextPlayerID, Name: name, Tiebreak: rng.Int64()})
	t.NextPlayerID++
	return nil
}

func (t *Tournament) RenamePlayer(id int, name string) error {
	p := t.Player(id)
	if p == nil {
		return fmt.Errorf("no player with id %d", id)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("player name is required")
	}
	for _, o := range t.Players {
		if o.ID != id && strings.EqualFold(o.Name, name) {
			return fmt.Errorf("duplicate player name %q", name)
		}
	}
	p.Name = name
	return nil
}

func (t *Tournament) SetDropped(id int, dropped bool) error {
	p := t.Player(id)
	if p == nil {
		return fmt.Errorf("no player with id %d", id)
	}
	p.Dropped = dropped
	return nil
}

// UpdateSettings changes the number of Swiss rounds and the top cut size.
func (t *Tournament) UpdateSettings(numRounds, topCut int) error {
	if numRounds < 1 || numRounds < len(t.Rounds) {
		return fmt.Errorf("number of rounds must be at least %d", max(1, len(t.Rounds)))
	}
	if t.Playoff != nil && topCut != t.TopCut {
		return errors.New("cannot change the top cut once it has started")
	}
	if !validTopCut(topCut) {
		return fmt.Errorf("top cut must be 0, 2, 4, 8 or 16 (got %d)", topCut)
	}
	t.NumRounds, t.TopCut = numRounds, topCut
	return nil
}

func (t *Tournament) SetDisplay(mode string) error {
	for _, m := range DisplayModes {
		if m == mode {
			t.Display = mode
			return nil
		}
	}
	return fmt.Errorf("unknown display mode %q", mode)
}

func (t *Tournament) CurrentRound() *Round {
	if len(t.Rounds) == 0 {
		return nil
	}
	return t.Rounds[len(t.Rounds)-1]
}

func (t *Tournament) Phase() string {
	switch {
	case t.Playoff != nil && t.Playoff.Champion() != 0:
		return PhaseComplete
	case t.Playoff != nil:
		return PhasePlayoff
	case len(t.Rounds) == 0:
		return PhaseRegistration
	case len(t.Rounds) >= t.NumRounds && t.CurrentRound().Complete():
		if t.TopCut == 0 {
			return PhaseComplete
		}
		return PhaseSwissDone
	}
	return PhaseSwiss
}

// ReportResult records game wins/draws for a match. Round numbers are 1-based,
// match indexes are 0-based positions within the round.
func (t *Tournament) ReportResult(stage string, round, match, p1Wins, p2Wins, draws int) error {
	m, err := t.match(stage, round, match)
	if err != nil {
		return err
	}
	if m.IsBye() {
		return errors.New("cannot change the result of a bye")
	}
	if p1Wins < 0 || p2Wins < 0 || draws < 0 || p1Wins > maxGameWins || p2Wins > maxGameWins || draws > maxGameDraws {
		return errors.New("invalid game counts")
	}
	if stage == StagePlayoff {
		if p1Wins == p2Wins {
			return errors.New("top cut matches must have a winner")
		}
		if round < len(t.Playoff.Rounds) && t.Playoff.Rounds[round].Reported() > 0 && m.Reported {
			// Changing the winner would invalidate later results.
			old := m.Winner()
			newWinner := m.P1
			if p2Wins > p1Wins {
				newWinner = m.P2
			}
			if old != newWinner {
				return errors.New("clear results in later top cut rounds before changing this winner")
			}
		}
	} else if t.Playoff != nil {
		return errors.New("cannot change Swiss results once the top cut has started")
	}
	m.P1Wins, m.P2Wins, m.Draws, m.Reported = p1Wins, p2Wins, draws, true
	if stage == StagePlayoff && t.Playoff.advance() {
		t.Timer.Reset()
	}
	return nil
}

func (t *Tournament) ClearResult(stage string, round, match int) error {
	m, err := t.match(stage, round, match)
	if err != nil {
		return err
	}
	if m.IsBye() {
		return errors.New("cannot clear a bye")
	}
	if stage == StagePlayoff {
		if round < len(t.Playoff.Rounds) && t.Playoff.Rounds[round].Reported() > 0 {
			return errors.New("clear results in later top cut rounds first")
		}
	} else if t.Playoff != nil {
		return errors.New("cannot change Swiss results once the top cut has started")
	}
	m.clearResult()
	if stage == StagePlayoff && t.Playoff.advance() {
		t.Timer.Reset()
	}
	return nil
}

func (t *Tournament) match(stage string, round, match int) (*Match, error) {
	var rounds []*Round
	switch stage {
	case StageSwiss:
		rounds = t.Rounds
	case StagePlayoff:
		if t.Playoff == nil {
			return nil, errors.New("top cut has not started")
		}
		rounds = t.Playoff.Rounds
	default:
		return nil, fmt.Errorf("unknown stage %q", stage)
	}
	if round < 1 || round > len(rounds) {
		return nil, fmt.Errorf("no round %d", round)
	}
	r := rounds[round-1]
	if match < 0 || match >= len(r.Matches) {
		return nil, fmt.Errorf("no match %d in round %d", match, round)
	}
	return r.Matches[match], nil
}

// DeleteLastRound removes the most recent Swiss round (e.g. to re-pair it).
func (t *Tournament) DeleteLastRound() error {
	if t.Playoff != nil {
		return errors.New("cannot delete Swiss rounds once the top cut has started")
	}
	if len(t.Rounds) == 0 {
		return errors.New("no rounds to delete")
	}
	t.Rounds = t.Rounds[:len(t.Rounds)-1]
	t.Timer.Reset()
	return nil
}

// SwapPlayers exchanges the seats of two players in the current Swiss round.
// Neither player's match may have a result reported (byes excepted).
func (t *Tournament) SwapPlayers(a, b int) error {
	if t.Playoff != nil {
		return errors.New("cannot change Swiss pairings once the top cut has started")
	}
	r := t.CurrentRound()
	if r == nil {
		return errors.New("no round has been paired")
	}
	if a == b {
		return errors.New("choose two different players")
	}
	find := func(id int) (*Match, *int) {
		for _, m := range r.Matches {
			if m.P1 == id {
				return m, &m.P1
			}
			if m.P2 == id {
				return m, &m.P2
			}
		}
		return nil, nil
	}
	ma, pa := find(a)
	mb, pb := find(b)
	if ma == nil || mb == nil {
		return errors.New("both players must be in the current round")
	}
	if ma == mb {
		return errors.New("those players are already paired against each other")
	}
	if (ma.Reported && !ma.IsBye()) || (mb.Reported && !mb.IsBye()) {
		return errors.New("clear the reported results before swapping")
	}
	*pa, *pb = *pb, *pa
	return nil
}

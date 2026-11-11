package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Invictus321/swissrounds/internal/tournament"
)

// Server holds the single tournament being run and persists it to disk
// after every change.
type Server struct {
	mu   sync.Mutex
	path string
	t    *tournament.Tournament
	rng  *rand.Rand
}

func NewServer(path string) (*Server, error) {
	s := &Server{
		path: path,
		rng:  rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), rand.Uint64())),
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, err
	}
	var t tournament.Tournament
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	s.t = &t
	return s, nil
}

func (s *Server) save() error {
	if s.t == nil {
		return nil
	}
	data, err := json.MarshalIndent(s.t, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// archive moves the current data file aside so a new event can start.
func (s *Server) archive() error {
	if _, err := os.Stat(s.path); err != nil {
		return nil
	}
	return os.Rename(s.path, fmt.Sprintf("%s.%s.bak", s.path, time.Now().Format("20060102-150405")))
}

func (s *Server) Routes(static fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", serveFile(static, "index.html"))
	mux.HandleFunc("GET /display", serveFile(static, "display.html"))

	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/standings.csv", s.handleStandingsCSV)
	mux.HandleFunc("POST /api/tournament", s.handleCreate)
	mux.HandleFunc("DELETE /api/tournament", s.handleReset)
	mux.HandleFunc("PATCH /api/tournament", s.handleSettings)
	mux.HandleFunc("POST /api/players", s.handleAddPlayer)
	mux.HandleFunc("PATCH /api/players/{id}", s.handleUpdatePlayer)
	mux.HandleFunc("POST /api/rounds", s.handlePairRound)
	mux.HandleFunc("DELETE /api/rounds/last", s.handleDeleteRound)
	mux.HandleFunc("POST /api/rounds/swap", s.handleSwap)
	mux.HandleFunc("POST /api/results", s.handleResult)
	mux.HandleFunc("POST /api/playoff", s.handleStartPlayoff)
	mux.HandleFunc("DELETE /api/playoff", s.handleCancelPlayoff)
	mux.HandleFunc("POST /api/timer", s.handleTimer)
	return mux
}

func serveFile(static fs.FS, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, static, name)
	}
}

type stateResponse struct {
	ServerNow      int64                  `json:"serverNow"`
	Tournament     *tournament.Tournament `json:"tournament"`
	Phase          string                 `json:"phase,omitempty"`
	Standings      []tournament.Standing  `json:"standings,omitempty"`
	StandingsRound int                    `json:"standingsRound"`
	// RoundPoints[i] maps player ID to match points going into round i+1.
	RoundPoints       []map[int]int `json:"roundPoints,omitempty"`
	PlayoffRoundNames []string      `json:"playoffRoundNames,omitempty"`
	Champion          int           `json:"champion,omitempty"`
}

func (s *Server) state() stateResponse {
	resp := stateResponse{ServerNow: time.Now().UnixMilli(), Tournament: s.t}
	t := s.t
	if t == nil {
		return resp
	}
	resp.Phase = t.Phase()
	resp.StandingsRound = t.CompletedRounds()
	resp.Standings = t.StandingsAfter(resp.StandingsRound)
	resp.RoundPoints = make([]map[int]int, len(t.Rounds))
	for i := range t.Rounds {
		resp.RoundPoints[i] = t.PointsBefore(i + 1)
	}
	if p := t.Playoff; p != nil {
		for i := 1; (p.Size >> i) >= 1; i++ {
			resp.PlayoffRoundNames = append(resp.PlayoffRoundNames, p.RoundName(i))
		}
		resp.Champion = p.Champion()
	}
	return resp
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// mutate decodes the request body into body (if non-nil), applies fn to the
// tournament under lock, saves, and responds with the new state.
func (s *Server) mutate(w http.ResponseWriter, r *http.Request, body any, fn func(t *tournament.Tournament) error) {
	if body != nil {
		if err := json.NewDecoder(r.Body).Decode(body); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.t == nil {
		writeError(w, http.StatusConflict, errors.New("no tournament has been created"))
		return
	}
	if err := fn(s.t); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.save(); err != nil {
		log.Printf("save failed: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Errorf("saving tournament: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, s.state())
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.state())
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string   `json:"name"`
		Players      []string `json:"players"`
		NumRounds    int      `json:"numRounds"`
		TopCut       int      `json:"topCut"`
		RoundMinutes int      `json:"roundMinutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := tournament.New(tournament.Config{
		Name: req.Name, Players: req.Players, NumRounds: req.NumRounds,
		TopCut: req.TopCut, RoundMinutes: req.RoundMinutes,
	}, s.rng)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.archive(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.t = t
	if err := s.save(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.state())
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.archive(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.t = nil
	writeJSON(w, http.StatusOK, s.state())
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         *string `json:"name"`
		NumRounds    *int    `json:"numRounds"`
		TopCut       *int    `json:"topCut"`
		RoundMinutes *int    `json:"roundMinutes"`
		Display      *string `json:"display"`
		Announcement *string `json:"announcement"`
	}
	s.mutate(w, r, &req, func(t *tournament.Tournament) error {
		if req.NumRounds != nil || req.TopCut != nil {
			n, c := t.NumRounds, t.TopCut
			if req.NumRounds != nil {
				n = *req.NumRounds
			}
			if req.TopCut != nil {
				c = *req.TopCut
			}
			if err := t.UpdateSettings(n, c); err != nil {
				return err
			}
		}
		if req.RoundMinutes != nil {
			if *req.RoundMinutes < 1 || *req.RoundMinutes > 600 {
				return errors.New("round length must be between 1 and 600 minutes")
			}
			t.Timer.SetDuration(int64(*req.RoundMinutes) * 60_000)
		}
		if req.Display != nil {
			if err := t.SetDisplay(*req.Display); err != nil {
				return err
			}
		}
		if req.Name != nil && *req.Name != "" {
			t.Name = *req.Name
		}
		if req.Announcement != nil {
			t.Announcement = *req.Announcement
		}
		return nil
	})
}

func (s *Server) handleAddPlayer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	s.mutate(w, r, &req, func(t *tournament.Tournament) error {
		return t.AddPlayer(req.Name, s.rng)
	})
}

func (s *Server) handleUpdatePlayer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid player id"))
		return
	}
	var req struct {
		Name    *string `json:"name"`
		Dropped *bool   `json:"dropped"`
	}
	s.mutate(w, r, &req, func(t *tournament.Tournament) error {
		if req.Name != nil {
			if err := t.RenamePlayer(id, *req.Name); err != nil {
				return err
			}
		}
		if req.Dropped != nil {
			return t.SetDropped(id, *req.Dropped)
		}
		return nil
	})
}

func (s *Server) handlePairRound(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, nil, func(t *tournament.Tournament) error {
		_, err := t.PairNextRound(s.rng)
		return err
	})
}

func (s *Server) handleDeleteRound(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, nil, func(t *tournament.Tournament) error { return t.DeleteLastRound() })
}

func (s *Server) handleSwap(w http.ResponseWriter, r *http.Request) {
	var req struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	s.mutate(w, r, &req, func(t *tournament.Tournament) error { return t.SwapPlayers(req.A, req.B) })
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stage  string `json:"stage"`
		Round  int    `json:"round"`
		Match  int    `json:"match"`
		P1Wins int    `json:"p1Wins"`
		P2Wins int    `json:"p2Wins"`
		Draws  int    `json:"draws"`
		Clear  bool   `json:"clear"`
	}
	s.mutate(w, r, &req, func(t *tournament.Tournament) error {
		if req.Clear {
			return t.ClearResult(req.Stage, req.Round, req.Match)
		}
		return t.ReportResult(req.Stage, req.Round, req.Match, req.P1Wins, req.P2Wins, req.Draws)
	})
}

func (s *Server) handleStartPlayoff(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, nil, func(t *tournament.Tournament) error { return t.StartPlayoff() })
}

func (s *Server) handleCancelPlayoff(w http.ResponseWriter, r *http.Request) {
	s.mutate(w, r, nil, func(t *tournament.Tournament) error { return t.CancelPlayoff() })
}

func (s *Server) handleTimer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action  string `json:"action"` // start, pause, reset, adjust
		Seconds int64  `json:"seconds"`
	}
	s.mutate(w, r, &req, func(t *tournament.Tournament) error {
		now := time.Now()
		switch req.Action {
		case "start":
			t.Timer.Start(now)
		case "pause":
			t.Timer.Pause(now)
		case "reset":
			t.Timer.Reset()
		case "adjust":
			t.Timer.Adjust(req.Seconds * 1000)
		default:
			return fmt.Errorf("unknown timer action %q", req.Action)
		}
		return nil
	})
}

func (s *Server) handleStandingsCSV(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.t == nil {
		writeError(w, http.StatusNotFound, errors.New("no tournament has been created"))
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="standings.csv"`)
	cw := csv.NewWriter(w)
	cw.Write([]string{"Rank", "Player", "Finish", "Points", "Wins", "Losses", "Draws", "OMW%", "GW%", "OGW%", "Dropped"})
	pct := func(f float64) string { return strconv.FormatFloat(f*100, 'f', 2, 64) }
	for _, st := range s.t.StandingsAfter(s.t.CompletedRounds()) {
		cw.Write([]string{
			strconv.Itoa(st.Rank), st.Name, st.Finish, strconv.Itoa(st.Points),
			strconv.Itoa(st.Wins), strconv.Itoa(st.Losses), strconv.Itoa(st.Draws),
			pct(st.OMW), pct(st.GWP), pct(st.OGW), strconv.FormatBool(st.Dropped),
		})
	}
	cw.Flush()
}

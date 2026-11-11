package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func do(t *testing.T, h http.Handler, method, url, body string) stateResponse {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, url, rec.Code, rec.Body.String())
	}
	var st stateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestServerPersistsTournament(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.json")
	static := fstest.MapFS{}
	s, err := NewServer(path)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Routes(static)
	do(t, h, "POST", "/api/tournament", `{"name":"Test","players":["A","B","C","D"],"topCut":0}`)
	do(t, h, "POST", "/api/rounds", "")
	st := do(t, h, "POST", "/api/results", `{"stage":"swiss","round":1,"match":0,"p1Wins":2,"p2Wins":1}`)
	if !st.Tournament.Rounds[0].Matches[0].Reported {
		t.Fatal("result not recorded")
	}

	// A new server reading the same file sees the same state.
	s2, err := NewServer(path)
	if err != nil {
		t.Fatal(err)
	}
	st2 := do(t, s2.Routes(static), "GET", "/api/state", "")
	if st2.Tournament == nil || len(st2.Tournament.Rounds) != 1 || !st2.Tournament.Rounds[0].Matches[0].Reported {
		t.Fatalf("tournament not restored from disk: %+v", st2.Tournament)
	}
}

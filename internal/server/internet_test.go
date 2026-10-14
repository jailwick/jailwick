package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInternetBadRequests(t *testing.T) {
	s := testServer(t, "")
	put := func(url, body string) int {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("PUT", url, strings.NewReader(body)))
		return w.Code
	}
	// Checked before the runtime is asked anything (there is none in tests).
	for body, want := range map[string]int{
		`{"minutes": -5}`:   http.StatusBadRequest,
		`{"minutes": 3000}`: http.StatusBadRequest, // over a day
		`nope`:              http.StatusBadRequest,
	} {
		if got := put("/api/projects/api/sessions/abc123/internet", body); got != want {
			t.Errorf("%s: got %d, want %d", body, got, want)
		}
	}
	if got := put("/api/projects/nope/sessions/abc123/internet", `{"minutes": 15}`); got != http.StatusNotFound {
		t.Errorf("unknown project: got %d", got)
	}
}

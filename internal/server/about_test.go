package server

import "testing"

func TestAbout(t *testing.T) {
	s := testServer(t, "")
	var v AboutView
	if code := call(t, s, "GET", "/api/about", ``, &v); code != 200 || v.Version != "test" || v.Projects != 1 || v.Folders["config"] == "" {
		t.Fatalf("%d %+v", code, v)
	}
}

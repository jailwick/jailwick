package proxy

import (
	"slices"
	"testing"
	"time"
)

func TestMergeDenied(t *testing.T) {
	t1, t2 := time.Unix(100, 0), time.Unix(200, 0)
	all := map[string]*Denied{}
	a := map[string]*Denied{"x.example": {Host: "x.example", Count: 2, Last: t1, Projects: []string{"api"}}}
	b := map[string]*Denied{"x.example": {Host: "x.example", Count: 3, Last: t2, Projects: []string{"web"}}}
	MergeDenied(all, a)
	MergeDenied(all, b)
	d := all["x.example"]
	if d.Count != 5 || !d.Last.Equal(t2) || !slices.Equal(d.Projects, []string{"api", "web"}) {
		t.Fatalf("%+v", d)
	}
	if a["x.example"].Count != 2 {
		t.Fatal("merging changed its source")
	}
}

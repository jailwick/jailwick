package image

import (
	"slices"
	"testing"

	"github.com/jailwick/jailwick/internal/runtime"
)

func TestUsage(t *testing.T) {
	imgs := []runtime.Image{
		{Ref: "jailwick-proxy:aaa", ID: "111111111111"},
		{Ref: "jailwick-base:bbb", ID: "222222222222"},    // the base of a project's sandbox
		{Ref: "jailwick-sandbox:old", ID: "333333333333"}, // settings changed since
		{Ref: "<none>:<none>", ID: "444444444444"},        // left by jw image update
		{Ref: "<none>:<none>", ID: "555555555555"},        // ... but a stopped container uses it
		{Ref: "jailwick-sandbox:new", ID: "666666666666"},
	}
	wanted := map[string][]string{
		"jailwick-proxy:aaa": {"api", "shop"}, "jailwick-base:bbb": {"api", "shop"}, "jailwick-sandbox:new": {"api"},
	}
	uses := Usage(imgs, wanted, []string{"sha256:5555555555550000000000000000000000000000000000000000000000000000"})
	var unused []string
	for _, u := range uses {
		if u.Unused() {
			unused = append(unused, u.ID)
		}
	}
	if !slices.Equal(unused, []string{"333333333333", "444444444444"}) {
		t.Fatalf("unused: %v", unused)
	}
	if !uses[3].Dangling() || uses[2].Dangling() || !uses[4].Containers || uses[0].Projects[1] != "shop" {
		t.Fatalf("uses: %+v", uses)
	}
}

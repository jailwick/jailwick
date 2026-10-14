package agent

import (
	"testing"

	"github.com/jailwick/jailwick/internal/agentapi"
)

func TestDBName(t *testing.T) {
	if got := DBName("app", "Feature/Billing-2"); got != "app_feature_billing_2" {
		t.Fatalf("got %q", got)
	}
	long := DBName("app", string(make([]byte, 100)))
	if len(long) != 63 {
		t.Fatalf("not truncated: %d", len(long))
	}
}

func TestDBPairs(t *testing.T) {
	c := &agentapi.Config{Postgres: &agentapi.Postgres{DB: "app", ExtraDBs: []string{"app_test", "other"}}}
	got := DBPairs(c, "auth")
	want := [][2]string{{"app", "app_auth"}, {"app_test", "app_auth_test"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v", got)
	}
	if DBPairs(&agentapi.Config{}, "x") != nil {
		t.Fatal("no postgres: no pairs")
	}
}

func TestRewriteDBNames(t *testing.T) {
	pairs := [][2]string{{"app", "app_wt"}, {"app_test", "app_wt_test"}}
	in := `DATABASE_URL=postgres://app:secret@localhost:5432/app
TEST_URL="postgresql://app:secret@localhost:5432/app_test?sslmode=disable"
DB_NAME=app
POSTGRES_DB = "app"
OTHER=app
APP_USER=app
QUERY_URL=postgres://u@h/application`
	want := `DATABASE_URL=postgres://app:secret@localhost:5432/app_wt
TEST_URL="postgresql://app:secret@localhost:5432/app_wt_test?sslmode=disable"
DB_NAME=app_wt
POSTGRES_DB = "app_wt"
OTHER=app
APP_USER=app
QUERY_URL=postgres://u@h/application`
	if got := RewriteDBNames(in, pairs); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFolderAndRegistry(t *testing.T) {
	r := &registry{}
	n1, _ := r.claim("feature/billing")
	n2, _ := r.claim("auth")
	if n1 != 1 || n2 != 2 {
		t.Fatalf("slots %d %d", n1, n2)
	}
	if s, b := r.find("feature-billing"); s != 1 || b != "feature/billing" {
		t.Fatalf("folder lookup failed: %d %q", s, b)
	}
	r.Slots[0] = ""
	if n, _ := r.claim("new"); n != 1 {
		t.Fatalf("freed slot not reused: %d", n)
	}
}

func TestWorktreeOf(t *testing.T) {
	if got := WorktreeOf("/r", "/r/.jailwick/worktrees/auth/src"); got != "auth" {
		t.Fatalf("got %q", got)
	}
	// Worktrees made before 0.2.5 are still recognised.
	if got := WorktreeOf("/r", "/r/.claude/worktrees/old"); got != "old" {
		t.Fatalf("got %q", got)
	}
	if got := WorktreeOf("/r", "/r/src"); got != "" {
		t.Fatalf("got %q", got)
	}
}

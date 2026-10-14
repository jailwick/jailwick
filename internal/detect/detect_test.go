package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, dir, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExampleEnvOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".env.example", "DATABASE_URL=postgres://dev:devpw@localhost:5433/shop\nREDIS_URL=redis://:r3dis@localhost:6379/0\n")

	s := Repo(dir)
	if !slices.Equal(s.Services, []string{"postgres", "redis"}) {
		t.Fatalf("services: %v", s.Services)
	}
	if s.Postgres.User != "dev" || s.Postgres.Password != "devpw" || s.Postgres.DB != "shop" || s.RedisPass != "r3dis" {
		t.Fatalf("logins: %+v %q", s.Postgres, s.RedisPass)
	}
	// Every localhost URL with a port needs a forward: inside the sandbox the service is another host.
	if !slices.Equal(s.Forwards, []string{"5433:postgres:5432", "6379:redis:6379"}) {
		t.Fatalf("forwards: %v", s.Forwards)
	}
	if len(s.EnvFiles) != 0 {
		t.Fatalf("an example file is never copied into worktrees: %v", s.EnvFiles)
	}
}

func TestRealEnvWinsOverExample(t *testing.T) {
	dir := t.TempDir()
	// The example sorts first in the walk; the real file's login must still win.
	write(t, dir, ".env.example", "DATABASE_URL=postgres://user:password@localhost:5432/example\nMONGO_URL=mongodb://m:mpw@localhost:27017/x\n")
	write(t, dir, ".env.local", "DATABASE_URL=postgres://app:secret@localhost:5432/real\n")

	s := Repo(dir)
	if s.Postgres.User != "app" || s.Postgres.DB != "real" {
		t.Fatalf("real env should win: %+v", s.Postgres)
	}
	if !slices.Equal(s.Services, []string{"mongo", "postgres"}) || s.MongoUser != "m" {
		t.Fatalf("services the example adds still count: %v %q", s.Services, s.MongoUser)
	}
}

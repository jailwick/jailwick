package detect

import (
	"slices"
	"testing"
)

func TestEnvOf(t *testing.T) {
	type pg = struct{ User, Password, DB string }
	cases := []struct {
		name     string
		vars     map[string]string
		services []string
		forwards []string
		pg       pg
		redis    string
		mongo    [2]string
	}{
		{
			name:     "url",
			vars:     map[string]string{"DATABASE_URL": "postgres://dev:pw@localhost:5433/shop"},
			services: []string{"postgres"}, forwards: []string{"5433:postgres:5432"},
			pg: pg{"dev", "pw", "shop"},
		},
		{
			name:     "split postgres variables",
			vars:     map[string]string{"POSTGRES_USER": "u", "POSTGRES_PASSWORD": "p", "POSTGRES_DB": "d", "POSTGRES_HOST": "localhost", "POSTGRES_PORT": "5432"},
			services: []string{"postgres"}, forwards: []string{"5432:postgres:5432"},
			pg: pg{"u", "p", "d"},
		},
		{
			name:     "generic DB_ keys of a Postgres app",
			vars:     map[string]string{"DB_CONNECTION": "pgsql", "DB_HOST": "127.0.0.1", "DB_USERNAME": "lara", "DB_PASSWORD": "secret", "DB_DATABASE": "shop"},
			services: []string{"postgres"},
			pg:       pg{"lara", "secret", "shop"},
		},
		{
			name: "generic DB_ keys of a MySQL app",
			vars: map[string]string{"DB_CONNECTION": "mysql", "DB_HOST": "127.0.0.1", "DB_USERNAME": "lara", "DB_PASSWORD": "secret", "DB_DATABASE": "shop"},
		},
		{
			name: "remote database",
			vars: map[string]string{"PGHOST": "db.example.com", "PGUSER": "u", "PGPASSWORD": "p", "PGDATABASE": "d"},
		},
		{
			name:     "service name as host needs no forward",
			vars:     map[string]string{"PGHOST": "postgres", "PGPORT": "5432", "PGUSER": "u", "PGDATABASE": "d"},
			services: []string{"postgres"},
			pg:       pg{"u", "", "d"},
		},
		{
			name:     "url wins, split fills the gaps",
			vars:     map[string]string{"DATABASE_URL": "postgres://url@localhost/urldb", "POSTGRES_USER": "split", "POSTGRES_PASSWORD": "pw"},
			services: []string{"postgres"},
			pg:       pg{"url", "pw", "urldb"},
		},
		{
			name:     "redis and mongo",
			vars:     map[string]string{"REDIS_PASSWORD": "r", "MONGO_INITDB_ROOT_USERNAME": "m", "MONGO_INITDB_ROOT_PASSWORD": "mp"},
			services: []string{"redis", "mongo"},
			redis:    "r", mongo: [2]string{"m", "mp"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := EnvOf(c.vars)
			if !slices.Equal(e.Services, c.services) || !slices.Equal(e.Forwards, c.forwards) {
				t.Errorf("services %v forwards %v, want %v %v", e.Services, e.Forwards, c.services, c.forwards)
			}
			if e.Postgres != c.pg || e.RedisPass != c.redis || e.MongoUser != c.mongo[0] || e.MongoPass != c.mongo[1] {
				t.Errorf("logins %+v", e.Logins)
			}
		})
	}
}

func TestLoginsFrom(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".env.example", "DB_CONNECTION=pgsql\nDB_USERNAME=ex\nDB_DATABASE=ex\n")
	write(t, dir, ".env", "POSTGRES_USER=real\nPOSTGRES_DB=real\n")

	s := Repo(dir)
	if s.Postgres.User != "real" || s.LoginsFrom != ".env" {
		t.Fatalf("the real file's login wins: %+v from %q", s.Postgres, s.LoginsFrom)
	}
	if !slices.Equal(s.EnvCandidates, []string{".env", ".env.example"}) {
		t.Fatalf("candidates: %v", s.EnvCandidates)
	}
}

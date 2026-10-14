package detect

import (
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jailwick/jailwick/internal/envfile"
)

// Logins are the service logins an app uses, as its env files give them.
type Logins struct {
	Postgres  struct{ User, Password, DB string }
	RedisPass string
	MongoUser string
	MongoPass string
}

// EnvFacts is what one env file says: the services it reaches on localhost, the forwards their
// ports need, and the logins.
type EnvFacts struct {
	Services []string
	Forwards []string // localport:service:port, for each local port the file names
	Logins
}

// splitKeys are the variables that hold a login in parts, by service and part, most specific first.
// The generic DB_* names count for Postgres only when the file says the database is Postgres (see
// LoginsOf), so a MySQL app's login isn't taken for one.
var splitKeys = map[string]map[string][]string{
	"postgres": {
		"user":     {"POSTGRES_USER", "PGUSER", "DATABASE_USER", "DATABASE_USERNAME", "DB_USER", "DB_USERNAME"},
		"password": {"POSTGRES_PASSWORD", "PGPASSWORD", "DATABASE_PASSWORD", "DB_PASSWORD", "DB_PASS"},
		"db":       {"POSTGRES_DB", "PGDATABASE", "DATABASE_NAME", "DB_NAME", "DB_DATABASE"},
		"host":     {"POSTGRES_HOST", "PGHOST", "DATABASE_HOST", "DB_HOST"},
		"port":     {"POSTGRES_PORT", "PGPORT", "DATABASE_PORT", "DB_PORT"},
	},
	"redis": {
		"password": {"REDIS_PASSWORD"},
		"host":     {"REDIS_HOST"},
		"port":     {"REDIS_PORT"},
	},
	"mongo": {
		"user":     {"MONGO_USER", "MONGODB_USER", "MONGO_INITDB_ROOT_USERNAME"},
		"password": {"MONGO_PASSWORD", "MONGODB_PASSWORD", "MONGO_INITDB_ROOT_PASSWORD"},
		"host":     {"MONGO_HOST", "MONGODB_HOST"},
		"port":     {"MONGO_PORT", "MONGODB_PORT"},
	},
}

// generic are the prefixes of keys that name no database (see saysPostgres).
var generic = []string{"DATABASE_", "DB_"}

// ReadEnv reads an env file's services and logins.
func ReadEnv(path string) (EnvFacts, error) {
	vars, err := envfile.Read(path)
	if err != nil {
		return EnvFacts{}, err
	}
	return EnvOf(vars), nil
}

// EnvOf finds the services and logins in an env file's variables: connection URLs first, then
// logins split over variables (DB_USER, POSTGRES_PASSWORD, ...), which only fill what a URL left
// empty. Services on other hosts are left out: the sandbox doesn't run those.
func EnvOf(vars map[string]string) EnvFacts {
	var e EnvFacts
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		for _, m := range urlRE.FindAllString(vars[k], -1) {
			if u, err := url.Parse(m); err == nil && isLocal(u.Hostname()) {
				e.addURL(u)
			}
		}
	}
	for _, svc := range []string{"postgres", "redis", "mongo"} {
		e.addSplit(svc, vars)
	}
	return e
}

func (e *EnvFacts) addURL(u *url.URL) {
	svc := service(u.Scheme)
	e.use(svc, u.Port())
	pw, _ := u.User.Password()
	switch svc {
	case "postgres":
		if e.Postgres.DB == "" && e.Postgres.User == "" {
			e.Postgres.User, e.Postgres.Password = u.User.Username(), pw
			e.Postgres.DB = strings.TrimPrefix(u.Path, "/")
		}
	case "redis":
		if e.RedisPass == "" {
			e.RedisPass = pw
		}
	case "mongo":
		if e.MongoUser == "" {
			e.MongoUser, e.MongoPass = u.User.Username(), pw
		}
	}
}

func (e *EnvFacts) addSplit(svc string, vars map[string]string) {
	keys := splitKeys[svc]
	get := func(part string) string {
		for _, k := range keys[part] {
			if v := vars[k]; v != "" && (svc != "postgres" || !isGeneric(k) || saysPostgres(vars)) {
				return v
			}
		}
		return ""
	}
	user, pw, db := get("user"), get("password"), get("db")
	if user == "" && pw == "" && db == "" {
		return
	}
	h := get("host")
	if h != "" && !isLocal(h) && h != host(svc) {
		return // a database elsewhere: not the sandbox's
	}
	port := get("port")
	if h == host(svc) {
		port = "" // already the service's own name: no forward
	}
	e.use(svc, port)
	switch svc {
	case "postgres":
		fill(&e.Postgres.User, user)
		fill(&e.Postgres.Password, pw)
		fill(&e.Postgres.DB, db)
	case "redis":
		fill(&e.RedisPass, pw)
	case "mongo":
		fill(&e.MongoUser, user)
		fill(&e.MongoPass, pw)
	}
}

// use records a service, and the forward a local port needs: inside the sandbox, localhost:<port>
// must reach the service, which is another host there.
func (e *EnvFacts) use(svc, port string) {
	if !slices.Contains(e.Services, svc) {
		e.Services = append(e.Services, svc)
	}
	p, _ := strconv.Atoi(port)
	if p == 0 {
		return
	}
	fwd := strconv.Itoa(p) + ":" + host(svc) + ":" + strconv.Itoa(defaultPort[svc])
	if !slices.Contains(e.Forwards, fwd) {
		e.Forwards = append(e.Forwards, fwd)
	}
}

// saysPostgres reports whether the file's database is Postgres, so its generic DB_* keys are its login.
func saysPostgres(vars map[string]string) bool {
	for _, k := range []string{"DB_CONNECTION", "DB_DRIVER", "DB_ENGINE", "DATABASE_DRIVER", "DATABASE_ENGINE"} {
		if v := strings.ToLower(vars[k]); strings.Contains(v, "pg") || strings.Contains(v, "postgres") {
			return true
		}
	}
	for k, v := range vars {
		if strings.HasPrefix(k, "POSTGRES_") || strings.HasPrefix(k, "PG") || strings.HasPrefix(strings.ToLower(v), "postgres") {
			return true
		}
	}
	return false
}

func isGeneric(key string) bool {
	for _, p := range generic {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

func fill(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}

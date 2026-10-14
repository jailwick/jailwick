package spec

import (
	"strings"

	"github.com/jailwick/jailwick/internal/config"
)

func dataService(name, image string) Container {
	return Container{
		Name: name, Image: image, NoNewPrivileges: true,
		Networks: []string{SandboxNetwork},
	}
}

func postgres(in Inputs) Container {
	pg := in.Project.Postgres
	c := dataService("postgres", pg.Image)
	c.Command = []string{"postgres", "-c", "timezone=" + pg.Timezone, "-c", "log_timezone=" + pg.Timezone}
	c.Env = map[string]string{
		"POSTGRES_USER": pg.User, "POSTGRES_PASSWORD": pg.Password, "POSTGRES_DB": pg.DB,
		"POSTGRES_EXTRA_DBS": extraDBs(pg.ExtraDBs),
		"TZ":                 pg.Timezone, "PGTZ": pg.Timezone,
	}
	// The parent folder works for both pg <= 17 (…/data) and pg 18+ (…/18/docker) layouts.
	c.Mounts = []Mount{
		{Type: Volume, Source: "pg-data", Target: "/var/lib/postgresql"},
		{Type: Bind, Source: in.PostgresInit, Target: "/docker-entrypoint-initdb.d", ReadOnly: true},
	}
	c.Health = &Health{Test: []string{"CMD-SHELL", "pg_isready -U " + pg.User + " -d " + pg.DB}, Interval: "5s", Retries: 10}
	return c
}

// extraDBs encodes extra databases for the init script: "name" or "name:user:password".
func extraDBs(dbs []config.ExtraDB) string {
	parts := make([]string, 0, len(dbs))
	for _, d := range dbs {
		if d.User == "" {
			parts = append(parts, d.Name)
		} else {
			parts = append(parts, d.Name+":"+d.User+":"+d.Password)
		}
	}
	return strings.Join(parts, " ")
}

func redis(in Inputs) Container {
	r := in.Project.Redis
	c := dataService("redis", r.Image)
	// Enough databases for every worktree slot (worktree n uses database n).
	cmd := []string{"redis-server", "--appendonly", "yes", "--databases", "128"}
	if len(r.Passwords) > 0 {
		// Every password is accepted for the default user, so dev and test env files can differ.
		cmd = append(cmd, "--user", "default", "on", "resetpass", "~*", "&*", "+@all")
		for _, pw := range r.Passwords {
			cmd = append(cmd, ">"+pw)
		}
		c.Env = map[string]string{"REDISCLI_AUTH": r.Passwords[0]}
	}
	c.Command = cmd
	c.Mounts = []Mount{{Type: Volume, Source: "redis-data", Target: "/data"}}
	c.Health = &Health{Test: []string{"CMD-SHELL", "redis-cli ping 2>/dev/null | grep -q PONG"}, Interval: "5s", Retries: 10}
	return c
}

// mongo runs a single-node replica set (rs0) with a root login, so transactions work. A replica
// set with auth needs a keyfile; it's generated once into its own volume.
func mongo(in Inputs) Container {
	m := in.Project.Mongo
	c := dataService("mongodb", m.Image)
	c.Env = map[string]string{"MONGO_INITDB_ROOT_USERNAME": m.User, "MONGO_INITDB_ROOT_PASSWORD": m.Password}
	c.Entrypoint = []string{"bash", "-c", `set -e
k=/data/keyfile/keyfile
if [ ! -s "$k" ]; then openssl rand -base64 756 > "$k"; chmod 400 "$k"; chown mongodb:mongodb "$k"; fi
exec docker-entrypoint.sh mongod --replSet rs0 --bind_ip_all --keyFile "$k" --wiredTigerCacheSizeGB 0.25`}
	c.Mounts = []Mount{
		{Type: Volume, Source: "mongo-data", Target: "/data/db"},
		{Type: Volume, Source: "mongo-keyfile", Target: "/data/keyfile"},
	}
	c.Health = &Health{
		Test:     []string{"CMD-SHELL", `mongosh --quiet -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval 'try { rs.status().ok } catch (e) { rs.initiate({_id: "rs0", members: [{_id: 0, host: "mongodb:27017"}]}).ok }' | grep -qx 1`},
		Interval: "5s", Retries: 30, StartPeriod: "60s",
	}
	return c
}

// Test services keep their data in memory and need no login.

func postgresTest(in Inputs) Container {
	p := in.Project
	size := "size=" + p.Test.TmpfsSize
	c := dataService("postgres-test", p.Test.PostgresImage)
	c.Command = []string{"postgres", "-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off",
		"-c", "timezone=" + p.Postgres.Timezone, "-c", "log_timezone=" + p.Postgres.Timezone}
	c.Env = map[string]string{"POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "test"}
	// Both paths: the images declare one of them as a volume, which would otherwise persist.
	c.Tmpfs = []string{"/var/lib/postgresql:" + size, "/var/lib/postgresql/data:" + size}
	c.Health = &Health{Test: []string{"CMD-SHELL", "pg_isready -U test -d test"}, Interval: "3s", Retries: 20}
	return c
}

func redisTest(in Inputs) Container {
	c := dataService("redis-test", in.Project.Test.RedisImage)
	c.Command = []string{"redis-server", "--save", "", "--appendonly", "no"}
	c.Tmpfs = []string{"/data"}
	c.Health = &Health{Test: []string{"CMD-SHELL", "redis-cli ping 2>/dev/null | grep -q PONG"}, Interval: "3s", Retries: 20}
	return c
}

func mongoTest(in Inputs) Container {
	p := in.Project
	c := dataService("mongodb-test", p.Test.MongoImage)
	c.Command = []string{"mongod", "--replSet", "rs0", "--bind_ip_all", "--wiredTigerCacheSizeGB", "0.25"}
	c.Tmpfs = []string{"/data/db:size=" + p.Test.TmpfsSize, "/data/configdb"}
	c.Health = &Health{
		Test:     []string{"CMD-SHELL", `mongosh --quiet --eval 'try { rs.status().ok } catch (e) { rs.initiate({_id: "rs0", members: [{_id: 0, host: "mongodb-test:27017"}]}).ok }' | grep -qx 1`},
		Interval: "3s", Retries: 30, StartPeriod: "30s",
	}
	return c
}

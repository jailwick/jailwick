package sandbox

import (
	"context"
	"net"
	"slices"
	"strings"

	"github.com/jailwick/jailwick/internal/agentapi"
)

// servicePorts are the ports another sandbox's services listen on.
var servicePorts = map[string]string{
	"postgres": "5432", "postgres-test": "5432",
	"redis": "6379", "redis-test": "6379",
	"mongodb": "27017", "mongodb-test": "27017",
}

// Verify runs the agent's isolation checks in a project's sandbox. The services of the other
// running sandboxes are passed along: the sandbox must not reach them.
func (m *Manager) Verify(ctx context.Context, project string) ([]agentapi.Check, error) {
	args := []string{"verify"}
	if others := m.otherServices(ctx, project); len(others) > 0 {
		args = append(args, "-other", strings.Join(others, ","))
	}
	var checks []agentapi.Check
	return checks, m.AgentJSON(ctx, MainContainer(project), &checks, args...)
}

// otherServices are host:port addresses of the services of every other running sandbox.
func (m *Manager) otherServices(ctx context.Context, project string) []string {
	by, err := m.RunningContainers(ctx)
	if err != nil {
		return nil
	}
	var out []string
	for p, cs := range by {
		if p == project {
			continue
		}
		for _, c := range cs {
			if port, ok := servicePorts[c.Service]; ok && c.Running() {
				out = append(out, net.JoinHostPort(c.Name, port))
			}
		}
	}
	slices.Sort(out)
	return out
}

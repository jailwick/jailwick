# Runtimes

jailwick talks to the container runtime through a driver (`internal/runtime`). Every driver must
keep the sandbox's guarantees (see [SECURITY.md](../SECURITY.md)); `jw verify` proves them on a
running sandbox.

| Driver | Runtimes | Status |
|---|---|---|
| `docker` | OrbStack, Docker Desktop, Colima (docker mode), Docker Engine on Linux | **Supported.** Tested on OrbStack (macOS 26, Docker 29.4, Compose 5.1). |
| `containerd` | nerdctl with Lima, Colima, Rancher Desktop or Finch in containerd mode; containerd on Linux | **Experimental, untested.** Same code path as docker (`nerdctl` takes the same commands and Compose files). Run `jw verify` after `jw up`. |
| `apple` | Apple `container` (macOS 26+) | **Not supported yet.** See below. |

Pick one with `runtime:` in `config.yaml` (`jw init` asks) or per project.

## Dev URLs

On OrbStack every container has a host name, so a server on port 3000 in the sandbox is at
`http://sandbox.jailwick-<project>.orb.local:3000`, and one a session starts is at
`http://session.jailwick-<project>--<id>.orb.local:3000` (`jw url`, `jw ports` and the app print
the right one). Other runtimes have no such names, and the
sandbox network is internal, so ports can't be published directly; the planned ingress forwarder
will publish them on `127.0.0.1`. Until then, servers in the sandbox are
reachable from inside it only on those runtimes.

## Networks per session

Each session has its own sandbox with its own network, on top of the
project's three. Docker's default address pools hold about 30 networks, so many sandboxes and
sessions at once can run out: starting a session then fails with "could not find an available,
non-overlapping IPv4 address pool". End some sessions (`jw stop-sessions`), or give the runtime
more pools (`default-address-pools` in Docker's `daemon.json`; OrbStack and Docker Desktop have it
in their engine settings).

## Apple `container` (findings, 2026-10-13, `container` 1.5)

Checked against the CLI reference and source (`apple/container`):

- **Host names between containers.** The sandbox reaches its services by name (`postgres`,
  `redis`, `egress-proxy`), and Compose gives each container its service name on the sandbox
  network. `container` has no network aliases and no `--add-host`; a container's name is its
  global id, so two projects can't both have a `postgres`. jailwick would need its own name
  resolution (e.g. a DNS server in the proxy container) before the sandbox works there.
- **Internal network still reaches the host.** `container network create --internal` is
  "host-only": no internet, but the Mac is reachable. jailwick requires the host to be
  unreachable, so the driver needs a way to block it (a packet filter rule for the vmnet
  subnet, or a different network plugin) before it can ship.
- **Missing options**, acceptable because each container is its own VM (a stronger boundary):
  `no-new-privileges`, PID limits. Documented, not blocking.
- **Available**: `--internal` networks, multiple `--network`, `--read-only`, `--cap-drop ALL`,
  `--user`, `--mount …,readonly`, `--tmpfs`, `--init`, `--shm-size`, named volumes, `container build`,
  JSON output for `list` and `inspect` (`[{id, configuration: {image, labels, …}, status: {state}}]`).

Next step: a spike on a Mac with `container` installed, solving name resolution and host
blocking, proven by `jw verify`.

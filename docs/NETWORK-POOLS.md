# "The container runtime has no network addresses left"

When a project or a session doesn't start and jailwick says the runtime has no network addresses
left (Docker's own words: *all predefined address pools have been fully subnetted*, or *could not
find an available, non-overlapping IPv4 address pool*), your container runtime has run out of
room for new networks. This page explains why, and how to give it more room, for each common
runtime.

## Why it happens

Every Docker network takes an address range from the runtime's **address pools**. With their
default settings, Docker-based runtimes have room for **about 30 networks**, shared by everything
on your machine: jailwick, your other Docker projects, and anything left behind by them.

jailwick keeps sandboxes apart with networks:

- **3 per project**: one for its services, one between the sandbox and its own proxy, and the
  proxy's way out;
- **1 per session**: each session's own proxy network, which lets one session's internet differ
  from the others'.

Four projects with a few sessions each use about 16 networks; add your other Docker projects and
the default room runs out.

## First aid (no settings changed)

- **Remove unused networks**: `docker network prune` removes every network no container uses.
- **Stop projects you don't use**: `jw down -p <name>` removes their networks too (their data
  stays).
- **Let jailwick check**: `jw doctor` counts the networks and lists jailwick's unused ones. When a
  start runs out of room, jailwick removes its own unused networks by itself and tries again.

These free room; the lasting fix is to give the runtime more address ranges, below.

## The lasting fix: more, smaller address ranges

All Docker-based runtimes take the same setting, `default-address-pools`, in the engine's
configuration (`daemon.json`). This one gives room for **256 networks**, each big enough for
hundreds of containers:

```json
{
  "default-address-pools": [
    { "base": "10.200.0.0/16", "size": 24 }
  ]
}
```

- `base` is the range the networks are cut from; `size` is the size of each network (`24`: 254
  addresses each). `10.200.0.0/16` cut into `/24` networks makes 256 of them.
- **Pick a range nothing else on your machine uses** (see [Choosing a range](#choosing-a-range)).
- Keep any other settings already in the file: add `default-address-pools` next to them.
- Networks that already exist keep their ranges; new ones use the new pools.

### OrbStack (macOS)

1. Open OrbStack → **Settings → Docker**, and edit the engine configuration (the JSON field), or
   edit `~/.orbstack/config/docker.json`.
2. Add `default-address-pools` as above, and apply. OrbStack restarts its Docker engine (or run
   `orb restart docker`).
3. Start the project again: `jw up`, or Start in the app.

### Docker Desktop (macOS, Windows, Linux)

1. Open Docker Desktop → **Settings → Docker Engine**.
2. Add `default-address-pools` to the JSON, as above.
3. Click **Apply & restart**.

### Colima (macOS, Linux)

1. Stop it and open its configuration: `colima stop`, then `colima start --edit` (or edit
   `~/.colima/default/colima.yaml`).
2. Under `docker:`, add the pools in YAML:
   ```yaml
   docker:
     default-address-pools:
       - base: 10.200.0.0/16
         size: 24
   ```
3. Save and close: Colima starts with the new setting.

### Docker Engine on Linux

1. Edit `/etc/docker/daemon.json` (create it if missing) and add `default-address-pools`, as
   above.
2. Restart Docker: `sudo systemctl restart docker`.

### Rancher Desktop (dockerd mode)

Rancher Desktop has no field for the engine's configuration; its documentation explains how to
provide a `daemon.json` through a provisioning script. Add `default-address-pools`, as above, to
that file.

### containerd (nerdctl)

jailwick's containerd support is experimental. nerdctl takes its networks' ranges from its CNI
configuration rather than `default-address-pools`; see nerdctl's documentation on network
subnets.

## Choosing a range

The range must not overlap networks your Mac or PC already reaches, or those addresses would
become unreachable from inside containers:

- **Your local network**: often `192.168.x.x`.
- **A VPN**: corporate VPNs often use parts of `10.0.0.0/8`.
- **Tailscale** and similar tools use `100.64.0.0/10`.

`10.200.0.0/16` is rarely used. If it clashes, pick another private range that's free, e.g.
`172.20.0.0/14` (room for 1024 `/24` networks). To see the ranges in use: `netstat -rn` (macOS)
or `ip route` (Linux).

## After the change

- Run `jw doctor`: it counts the networks and warns again when room runs short.
- Running sandboxes keep working; projects that couldn't start can be started again.

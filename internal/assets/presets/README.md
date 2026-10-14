# Allowlist presets

One file per preset: `<name>.yaml`. The name is what projects list in `allow_presets`.

```yaml
description: npm, yarn and pnpm packages        # one line, shown in jw proxy presets and the app
category: packages                              # agent | code | packages | toolchains | services | docs | editors
hosts: [registry.npmjs.org, .yarnpkg.com]       # .example.com: the domain and all its subdomains
detect:                                         # what in a repo suggests this preset
  files: [package.json, "*.csproj"]             #   file names or globs, anywhere 3 levels deep
  packages: ["@prisma/client", "@sentry/*"]     #   dependency names from manifests; * = prefix
  remotes: [github.com]                         #   hosts of the repo's git remotes
broad: "anyone can push to their own gists"     # set when the hosts accept data from anyone
docs: https://docs.npmjs.com
```

Adding one:

- Only hosts the tool needs to install or run, checked against its documentation. Prefer exact
  hosts; use `.domain` only when the subdomains are many or change.
- Set `broad` when anyone can upload to the hosts (storage buckets, gists, error trackers,
  pushes). Detection then suggests the preset but doesn't choose it.
- `docs` and `editors` presets have no `detect`: people choose them.
- `go test ./internal/presets` checks the format, overlaps, and that no preset admits
  `example.com` (verify expects it blocked).

Presets of your own (`jw proxy presets add`, or the app's Network page) live in
`<config>/presets/<name>.yaml` with only `description` and `hosts`; they show under "custom"
and projects add them like any other (`jw allow --preset <name>`).

// Typed client for jailwick's local API (internal/server). Same origin in the desktop app and
// in browser mode.

export type Runtime = { name: string; version?: string; platform?: string; arch?: string; urls: boolean; error?: string }

export type ProjectSummary = {
  name: string
  dir: string
  workspace?: string
  services: string[]
  running: boolean
  sessions: number
  busy: number
  error?: string
  build_failed?: boolean // the last start's image build failed
  op?: Op // a start, restart or stop under way (from Live)
  health?: Health // from Live
}

// Overview is the configuration alone (it answers at once); Live is what the runtime reports, by
// running project. The app shows them merged: a Dashboard.
export type Overview = { version: string; projects: ProjectSummary[]; workspaces: string[] }
export type Live = {
  runtime: Runtime
  projects: Record<string, { sessions: number; busy: number }>
  ops?: Record<string, Op>
  health?: Record<string, Health> // only the projects with something to show
}
// Problem: a container of a sandbox not doing its work (stopped, crashing, unhealthy, paused).
export type Problem = { service: string; state: string; health?: string; restarts?: number; exit_code?: number; crashed?: boolean; what: string }
// Health: the main container paused or restarting (it crashed: the runtime brings it back), and
// the containers not doing their work.
export type Health = { main?: 'paused' | 'restarting'; problems?: Problem[] }
// Op is what a project's sandbox is going through; building is its images alone.
export type Op = 'starting' | 'restarting' | 'stopping' | 'building'
// live: the runtime has answered (until then, running, sessions and busy aren't known).
export type Dashboard = Overview & { runtime?: Runtime; live: boolean }

export type Session = {
  pid: number
  dir: string
  worktree?: string
  started: string
  name?: string
  state?: string
  id?: string
  sid?: string // the session's own sandbox; absent for one in the project's container
  url?: string // reaches the servers the session starts (append :port)
  internet?: Internet // absent without a proxy of its own
  chrome?: boolean // the session may use Claude in Chrome (the user's own browser)
  private?: boolean // kept out of messages between sessions
  terminal?: string // the host terminal app it runs in, for Show terminal; absent when unknown
}

// What Show terminal brought forward: the session's own tab, the terminal app only, or nothing.
export type FocusResult = { found: 'tab' | 'app' | ''; app?: string; note?: string }

// Internet is whether a session's proxy lets it reach everything, and until when (none: until closed).
export type Internet = { open: boolean; until?: string }

export type Worktree = {
  name: string
  dir: string
  branch: string
  slot: number
  port: number
  dbs?: string[]
  redis_db: number
  missing?: boolean
  unmanaged?: boolean // no slot: shares the main checkout's ports and databases
}

// A worktree being made from the app, or made with a problem (failed, or its setup failed).
export type WorktreeJob = { name: string; base: string; running: boolean; error?: string; setup_failed?: boolean }

// What removing a worktree would take away. into: the branch "merged" means (the main checkout's).
export type WorktreeCheck = {
  name: string; branch: string; missing?: boolean; changed: number
  into?: string; merged: boolean; ahead: number; pushed: boolean; sessions: number
}

export type Container = { Name: string; Service: string; State: string; Health: string; ImageID: string }

export type ProjectConfig = {
  run?: string // the run command (Run button)
  dir: string
  workspace?: string
  services?: string[]
  test_services?: string[]
  local_forwards?: string[]
  image?: {
    base?: string; dockerfile?: string; name?: string; claude_code_version?: string; platform?: string; user?: string
    stacks?: Record<string, string> // name -> version ('' for the stack's default)
    playwright?: { browsers?: string[] }
    apt_packages?: string[]
    android?: boolean
  }
  resources?: { memory?: string; cpus?: string }
  claude?: { yolo?: boolean; login?: 'app' | 'project'; chrome?: boolean; profiles?: string[] }
  postgres?: { db?: string; user?: string; image?: string }
  redis?: { image?: string }
  mongo?: { image?: string }
  worktree?: { copy_files?: string[] }
  allow?: string[]
  allow_presets?: string[]
}

export type ProjectDetail = ProjectSummary & {
  op?: Op // under way, also when started from another page (or by jw)
  health: Health
  config: ProjectConfig
  containers: Container[]
  pending: string[] // one line per container
  changes: Change[] // the same, item by item
  session_list: Session[]
  worktrees: Worktree[]
  worktree_jobs: WorktreeJob[]
  url_base: string // the project's own container; sessions have their own url
  allow: string[]
  allow_groups: AllowGroup[] // where the allowlist comes from
  files: { config: string; notes: string }
  commands: { claude: string; shell: string } // the commands that start a session, to copy
  runs: Run[] // the run command, where it runs
  // node_modules kept apart from the Mac's: found automatically or the project's own list
  // (setting: folders or patterns); dirs below the top, empty: nothing installed yet ('.' the top).
  // used: the repo has a package.json, or folders are chosen (else there's nothing to keep apart)
  node_modules: { auto: boolean; setting: string[]; dirs: string[]; empty: string[]; used: boolean }
  build?: BuildStatus // an image being built for a start or restart, or the last one that failed
}

// One difference between a running container and what a restart would create: what (and which
// item of it), before and after, and notes in words (what changed in jailwick's agent). Secret
// values never appear; detail is for debugging (content hashes).
export type Change = {
  container: string
  what: string
  item?: string
  before?: string
  after?: string
  notes_title?: string
  notes?: string[]
  detail?: string
}

// A start's image build: the step, how far it is (done of total steps), its latest output line and
// its log file. A failed build keeps its last lines in tail.
export type BuildStatus = {
  step: string
  log: string
  line: string
  done?: number
  total?: number
  failed?: boolean
  tail?: string[]
}

// A sandbox whose project file is gone: no project, so its sessions are in no other list.
export type Leftover = {
  name: string
  created: string
  containers: { name: string; state: string }[]
  sessions: Session[]
}

export type SessionView = Session & { project: string }

// devBase is where the servers of a worktree ('' for the main checkout) are reached: its newest
// session's container, else the project's.
export function devBase(d: ProjectDetail, worktree = ''): string {
  const newestFirst = [...d.session_list].reverse() // the list is oldest first
  return newestFirst.find((s) => (s.worktree ?? '') === worktree && s.url)?.url ?? d.url_base
}
export type Check = { section: string; name: string; ok: boolean; info?: boolean; detail?: string }
export type Port = { port: number; address: string; pid?: number; command?: string; where: string; sid?: string; url?: string }
// What a checkout changed: a worktree against the main checkout's branch (base), from where they
// part; the main checkout against its last commit. diff: one file's, when asked.
export type ChangedFile = { path: string; status: string; added: number; deleted: number; binary?: boolean }
export type Changes = {
  base?: string
  head: string
  commits?: { hash: string; subject: string }[]
  files: ChangedFile[]
  diff?: string
  diff_cut?: boolean
}
// The project's run command in the main checkout ('' worktree) or a worktree; port: the worktree's.
export type Run = { worktree?: string; pid: number; started: string; running: boolean; port?: number }
// A saved conversation; command resumes it in the folder it ran in (to copy for any terminal).
export type Conversation = {
  id: string
  dir: string
  where: string
  when: string
  prompt: string
  name?: string // the name given with /rename
  gone?: boolean
  command?: string // resumes it: cd to its folder, then run
  run?: string // the same without the cd: jw claude <id>
  folder?: string // its folder, ~/…
}
// A session to open in the terminal: Claude or a shell, in the main checkout or a worktree, or a
// saved conversation resumed where it ran.
// private keeps a Claude session out of messages between sessions.
// command 'folder': a plain terminal in dir (no jailwick).
export type SessionRequest = { worktree?: string; command?: 'claude' | 'shell' | 'folder'; open?: boolean; private?: boolean; resume?: string; dir?: string }
export type Denied = { Host: string; Count: number; First?: string; Last: string; Projects: string[] }
// One session that asked for a refused host: its sandbox (session; none: the project's own, for
// shells and runs), worktree and Claude session name, how often and when. Kept after it ends.
export type BlockedList = { hosts: Denied[]; discarded: Denied[]; kept: string[]; by: Record<string, DeniedBy[]> }
export type DeniedBy = { session?: string; worktree?: string; name?: string; count: number; first: string; last: string }
// Preset is one group of hosts from the catalogue (internal/assets/presets). broad: why it can
// receive data from anyone.
export type Preset = { name: string; description: string; category: string; hosts: string[]; broad?: string; docs?: string; always?: boolean }
export type Network = {
  user: string[]
  presets: Preset[] // built-in, then the user's own (category 'custom')
  preset_problems?: string // preset files of the user's that can't be read
  projects: Record<string, string[]> // each project's own domains
  project_presets: Record<string, string[]> // each project's allow_presets
  blocked: Denied[]
}
// One source of a project's allowlist: a preset, the project's own domains, the user's
// allowlist.txt, config.yaml, or Claude in Chrome's relay.
export type AllowGroup = { kind: 'preset' | 'project' | 'user' | 'global' | 'chrome'; name: string; hosts: string[] }
export type Login = 'app' | 'project'
// A change to a project's Claude settings; what is left out stays. login '' follows the default.
export type ClaudeChange = { login?: Login | ''; chrome?: boolean; profiles?: string[] }
// Secrets: names and scopes only, never values.
export type Secrets = {
  store: string
  read_only: boolean // the env store: secrets come from the environment
  secrets: { name: string; project?: string }[]
  known: string[]
  overrides: string[] // names that replace Claude's login
}
// Version: the file as read; a change names it, and is refused (409) when a session changed the file since.
export type Handoff = { File: string; Inbox: string; From: string; Status: string; Created: string; Subject: string; Done: boolean; Version: string }
export type WorkspaceView = {
  config: { root: string; handoff?: string; root_notes?: string; exclude?: string[] }
  members: { name: string; dir: string; project?: string }[] // project: the one the folder is, if any
  projects: string[]
  handoffs: Handoff[]
  project?: string // whose view hidden is
  hidden: string[]
  rules: string[]
  error?: string
}
// A project's side of a workspace: which one, the members it sees (all when related is empty) and
// its own hiding rules; members and candidates (workspaces whose root holds it) are the choices.
export type Membership = { workspace: string; related: string[]; exclude: string[]; members: string[]; candidates: string[] }
// After a change: restart, the workspace's other projects that see the project's folder added or
// taken out of its members after a restart.
export type MembershipChange = Membership & { restart: string[] }
export type Notes = { text: string; path: string }
// What saving a workspace changed: the projects that joined or left (a restart applies it).
export type WorkspaceChange = { name?: string; changed: string[] }
// About this build: its version and agent, the machine and runtime, where jailwick keeps its files.
export type About = {
  version: string // e.g. 0.2.5
  build: { version: string; commit?: string; date?: string; modified?: boolean } // what it was built from
  agent?: { label: string; changes: { date: string; text: string }[] }
  os: string
  arch: string
  go: string
  runtime: Runtime
  folders: { config: string; state: string; data: string; build_logs: string }
  projects: number
  workspaces: number
}
// An image jailwick built; projects and containers say what keeps it (neither: unused).
// pulled: a project's own base or image from a registry, not one jailwick built (never pruned).
export type Image = { Ref: string; ID: string; Size: string; Created: string; projects: string[]; containers: boolean; pulled?: boolean }
// Where a session tells you it waits: its terminal where the terminal shows notifications, else
// the app (auto); always the terminal; always the app; nowhere.
// One thing an image check looked at (distro, libc, arch, user, home, claude, git).
export type ImageFinding = { name: string; ok: boolean; detail: string }
export type SessionNotify = 'auto' | 'terminal' | 'app' | 'off'
export type Settings = {
  global: { runtime?: string; terminal?: string; notifications?: boolean; image?: { base?: string; dockerfile?: string; name?: string; user?: string; home?: string }; session_notifications?: SessionNotify; blocked_notifications?: boolean; updates?: boolean; secrets?: string; git?: { name?: string; email?: string }; claude?: { yolo?: boolean }; allow?: string[] }
  config_dir: string
  state_dir: string
  secret_store: string
  secrets: string[]
  version: string
  profiles: string[] // Claude settings profiles, for claude.profiles
  login_default: Login // claude.login in config.yaml
  logins: { project: string; login: Login | ''; uses: Login }[] // '' : follows the default
}

// The guided "new project" flow (internal/projectsetup): steps of fields, prefilled from the repo.
export type FieldKind = 'text' | 'secret' | 'multi' | 'list' | 'choice'
export type FieldValue = string | string[]
export type SetupField = {
  key: string
  label: string
  help?: string
  placeholder?: string
  note?: string // about this draft, e.g. presets detected but not chosen
  kind: FieldKind
  options?: SetupOption[]
  show_if?: { key: string; has?: string; not?: boolean } // not: shown while it doesn't hold
  action?: string // something the app can do with the value, next to it ('check-image')
  resets?: string[] // keys a change to this field recomputes
  value: FieldValue
}
// SetupOption is one choice of a multi field (a suggestion for a text field): shown under its group,
// with a tag and a warning to think twice; fixed ones are always chosen, disabled ones can't be.
export type SetupOption = {
  value: string
  label?: string // how a choice shows it (else the value)
  help?: string
  group?: string
  tag?: string
  warn?: string
  fixed?: boolean
  disabled?: boolean
}
export type SetupStep = { id: string; title: string; fields: SetupField[] }
// A setup being answered (a project, a workspace): its folder, steps, hints, the file it would write
// and what still has to change.
export type SetupDraft = { dir: string; steps: SetupStep[]; hints: string[]; yaml: string; problems: string[] }
export type SetupValues = Record<string, FieldValue>

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message)
  }
}

// A published version, and what the daily check found (internal/update).
export type Release = { version: string; url: string; notes?: string; published: string }
export type UpdateStatus = {
  current: string
  latest?: Release // until a check succeeded
  available: boolean
  checked?: string
  error?: string // the last check failed
  install: { method: 'homebrew' | 'download'; command?: string } // how this jailwick updates
  off: boolean // updates: off (checks only when asked)
}
export type UpdateRun = { ok: boolean; output: string; error?: string }

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError((data as { error?: string }).error ?? res.statusText, res.status)
  return data as T
}

const p = (name: string) => `/api/projects/${encodeURIComponent(name)}`
const w = (name: string) => `/api/workspaces/${encodeURIComponent(name)}`

export const api = {
  overview: () => call<Overview>('GET', '/api/overview'),
  live: () => call<Live>('GET', '/api/live'),
  draftProject: (dir: string, values: SetupValues = {}) => call<SetupDraft>('POST', '/api/projects/draft', { dir, values }),
  // restart: the workspace's other projects, which see the new project's folder after a restart.
  createProject: (dir: string, values: SetupValues) =>
    call<{ name: string; workspace: string; restart: string[] }>('POST', '/api/projects', { dir, values }),
  // The system's folder dialog, opened in start; path is "" when cancelled.
  pickFolder: (start = '', title = '') => call<{ path: string }>('POST', '/api/pick-folder', { start, title }),
  project: (name: string) => call<ProjectDetail>('GET', p(name)),
  up: (name: string) => call<unknown>('POST', `${p(name)}/up`),
  restart: (name: string, force = false) => call<unknown>('POST', `${p(name)}/restart${force ? '?force=1' : ''}`),
  // Build the project's images again (like jw image build); fresh: no cache, the bases pulled again.
  rebuildImage: (name: string, fresh = false) => call<unknown>('POST', `${p(name)}/image/build${fresh ? '?fresh=1' : ''}`),
  startService: (name: string, service: string) => call<unknown>('POST', `${p(name)}/services/${encodeURIComponent(service)}/start`),
  down: (name: string, force = false) => call<unknown>('POST', `${p(name)}/down${force ? '?force=1' : ''}`),
  stopSessions: (name: string) => call<unknown>('POST', `${p(name)}/stop-sessions`),
  logs: (name: string, service = '', tail = 300) =>
    call<{ text: string }>('GET', `${p(name)}/logs?service=${encodeURIComponent(service)}&tail=${tail}`),
  verify: (name: string) => call<Check[]>('GET', `${p(name)}/verify`),
  ports: (name: string) => call<Port[]>('GET', `${p(name)}/ports`),
  // Hosts the project's sandbox was refused that its allowlist still doesn't cover.
  // hosts: refused and not on the allowlist, most often first; kept: hosts the project keeps
  // blocked, which aren't listed; by: which sessions asked for each host listed.
  // From the project's records: also after a restart, a stop or a reinstall. discarded: cleared
  // from the list, not refused since.
  blocked: (name: string) => call<BlockedList>('GET', `${p(name)}/blocked`),
  // Clear hosts from the list until the sandbox asks for them again (each with the last refusal
  // shown), or bring them back. Kept in the project's data.
  discard: (name: string, hosts: { host: string; last?: string }[], restore = false) =>
    call<unknown>('POST', `${p(name)}/discard`, { hosts, restore }),
  // Keep hosts blocked (no longer listed or notified), or stop keeping them.
  block: (name: string, hosts: string[], unblock = false) => call<{ kept: string[] }>('POST', `${p(name)}/block`, { hosts, unblock }),
  history: (name: string) => call<Conversation[]>('GET', `${p(name)}/history`),
  notes: (name: string) => call<Notes>('GET', `${p(name)}/notes`),
  saveNotes: (name: string, text: string) => call<unknown>('PUT', `${p(name)}/notes`, { text }),
  addWorktree: (name: string, wt: string, base: string) => call<unknown>('POST', `${p(name)}/worktrees`, { name: wt, base }),
  checkWorktree: (name: string, wt: string) => call<WorktreeCheck>('GET', `${p(name)}/worktree?wt=${encodeURIComponent(wt)}`),
  // force: with its uncommitted changes; branch: merged (only if merged) or any, to delete it too.
  removeWorktree: (name: string, wt: string, force: boolean, branch: '' | 'merged' | 'any') =>
    call<unknown>('DELETE', `${p(name)}/worktree?${new URLSearchParams({ wt, ...(force ? { force: '1' } : {}), ...(branch ? { branch } : {}) })}`),
  worktreeLog: (name: string, wt: string) => call<{ text: string }>('GET', `${p(name)}/worktree/log?wt=${encodeURIComponent(wt)}`),
  dismissWorktreeJob: (name: string, wt: string) => call<unknown>('DELETE', `${p(name)}/worktree/job?wt=${encodeURIComponent(wt)}`),
  branches: (name: string) => call<string[]>('GET', `${p(name)}/branches`),
  terminal: (name: string, s: SessionRequest = {}) => call<unknown>('POST', `${p(name)}/terminal`, s),
  run: (name: string, action: 'start' | 'stop', worktree = '') => call<Run[]>('POST', `${p(name)}/run`, { action, worktree }),
  changes: (name: string, worktree = '', file = '') =>
    call<Changes>('GET', `${p(name)}/changes?worktree=${encodeURIComponent(worktree)}&file=${encodeURIComponent(file)}`),
  saveNodeModules: (name: string, dirs: string[]) => call<unknown>('PUT', `${p(name)}/node-modules`, { dirs }),
  // An existing project's settings, as setup fields: services and logins, toolset, network,
  // worktrees, resources (all but the name and the workspace).
  draftSettings: (name: string, values: SetupValues = {}) => call<SetupDraft>('POST', `${p(name)}/settings/draft`, { values }),
  saveSettings: (name: string, values: SetupValues) => call<unknown>('PUT', `${p(name)}/settings`, { values }),
  // The running (or failed) build's output from byte from on; it says where it starts when that isn't from.
  buildLog: (name: string, from = 0) =>
    call<{ text: string; from: number; size: number }>('GET', `${p(name)}/build/log?from=${from}`),
  runLog: (name: string, worktree = '') =>
    call<{ text: string }>('GET', `${p(name)}/run/log?worktree=${encodeURIComponent(worktree)}`),
  // minutes: 0 keeps it open until closed; opening again sets a new end.
  openInternet: (name: string, sid: string, minutes: number) =>
    call<Internet>('PUT', `${p(name)}/sessions/${encodeURIComponent(sid)}/internet`, { minutes }),
  closeInternet: (name: string, sid: string) => call<unknown>('DELETE', `${p(name)}/sessions/${encodeURIComponent(sid)}/internet`),
  showTerminal: (name: string, sid: string) => call<FocusResult>('POST', `${p(name)}/sessions/${encodeURIComponent(sid)}/show`),
  endSession: (name: string, sid: string) => call<unknown>('POST', `${p(name)}/sessions/${encodeURIComponent(sid)}/end`),
  sessions: () => call<SessionView[]>('GET', '/api/sessions'),
  leftovers: () => call<Leftover[]>('GET', '/api/leftovers'),
  removeLeftover: (name: string, data: boolean) =>
    call<{ removed: string }>('DELETE', `/api/leftovers/${encodeURIComponent(name)}${data ? '?data=1' : ''}`),
  network: () => call<Network>('GET', '/api/network'),
  allowPreset: (preset: string, project: string) =>
    call<{ reloaded: string[] }>('POST', '/api/network/allow', { preset, project }),
  denyPreset: (preset: string, project: string) => call<unknown>('POST', '/api/network/deny', { preset, project }),
  // hosts: separated by commas, spaces or new lines.
  savePreset: (name: string, description: string, hosts: string) =>
    call<{ name: string; hosts: string[]; reloaded: string[] }>('PUT', `/api/presets/${encodeURIComponent(name)}`, { description, hosts }),
  presets: () => call<{ presets: Preset[]; problems?: string }>('GET', '/api/presets'),
  deletePreset: (name: string) => call<unknown>('DELETE', `/api/presets/${encodeURIComponent(name)}`),
  saveClaude: (name: string, c: ClaudeChange) => call<unknown>('PATCH', `${p(name)}/claude`, c),
  saveDefaultLogin: (login: Login) => call<unknown>('PATCH', '/api/settings/claude', { login }),
  saveTerminal: (terminal: string) => call<unknown>('PATCH', '/api/settings/terminal', { terminal }),
  // The notification settings given: the app's on or off, where sessions tell they wait, and
  // notifications about blocked hosts.
  saveNotifications: (change: { on?: boolean; sessions?: SessionNotify; blocked?: boolean }) =>
    call<unknown>('PATCH', '/api/settings/notifications', change),
  secrets: () => call<Secrets>('GET', '/api/secrets'),
  setSecret: (name: string, value: string, project = '') => call<unknown>('PUT', '/api/secrets', { name, value, project }),
  unsetSecret: (name: string, project = '') =>
    call<unknown>('DELETE', `/api/secrets/${encodeURIComponent(name)}${project ? `?project=${encodeURIComponent(project)}` : ''}`),
  allow: (domain: string, project = '') => call<{ domain: string; reloaded: string[] }>('POST', '/api/network/allow', { domain, project }),
  // Several domains at once: the proxies reload once.
  allowMany: (domains: string[], project = '') =>
    call<{ domain: string; reloaded: string[] }>('POST', '/api/network/allow', { domains, project }),
  deny: (domain: string, project = '') => call<unknown>('POST', '/api/network/deny', { domain, project }),
  workspace: (name: string, project = '') =>
    call<WorkspaceView>('GET', `${w(name)}${project ? `?project=${encodeURIComponent(project)}` : ''}`),
  workspaceNotes: (name: string) => call<Notes>('GET', `${w(name)}/notes`),
  saveWorkspaceNotes: (name: string, text: string) => call<unknown>('PUT', `${w(name)}/notes`, { text }),
  handoff: (name: string, file: string) => call<{ meta: Handoff; body: string; path: string }>('GET', `${w(name)}/handoff?file=${encodeURIComponent(file)}`),
  // A request's status (done moves it to done/, another status takes it back to its inbox); the file where it is now.
  setHandoffStatus: (name: string, h: Handoff, status: 'open' | 'answered' | 'done') =>
    call<{ file: string }>('PATCH', `${w(name)}/handoff`, { file: h.File, version: h.Version, status }),
  deleteHandoff: (name: string, h: Handoff) =>
    call<unknown>('DELETE', `${w(name)}/handoff?${new URLSearchParams({ file: h.File, version: h.Version })}`),
  // Opens the request's file in its default app, or (reveal) shows it in the Finder.
  openHandoff: (name: string, file: string, reveal = false) => call<unknown>('POST', `${w(name)}/handoff/open`, { file, reveal }),
  membership: (project: string) => call<Membership>('GET', `${p(project)}/workspace`),
  // show_folder, when joining: its folder becomes a member (the other projects see it); remove_folder,
  // when leaving: its folder stops being one (they no longer see it).
  setMembership: (project: string, m: Pick<Membership, 'workspace' | 'related' | 'exclude'> & { show_folder?: boolean; remove_folder?: boolean }) =>
    call<MembershipChange>('PUT', `${p(project)}/workspace`, m),
  // A new workspace is drafted from its root folder, an existing one by name.
  draftWorkspace: (at: { root: string } | { name: string }, values: SetupValues = {}) =>
    call<SetupDraft>('POST', '/api/workspaces/draft', { ...at, values }),
  createWorkspace: (root: string, values: SetupValues) => call<WorkspaceChange>('POST', '/api/workspaces', { root, values }),
  saveWorkspace: (name: string, values: SetupValues) => call<WorkspaceChange>('PUT', w(name), { values }),
  removeWorkspace: (name: string) => call<WorkspaceChange>('DELETE', w(name)),
  images: () => call<Image[]>('GET', '/api/images'),
  // Tries an image for jailwick: Debian or Ubuntu with glibc, the architecture, the user and home;
  // used as is, Claude Code and git too. Pulls it first, so it can take a while.
  checkImage: (o: { ref: string; user?: string; home?: string; as_is?: boolean; platform?: string }) =>
    call<{ ok: boolean; findings: ImageFinding[] }>('POST', '/api/images/check', o),
  // The image projects build on unless they name their own (empty base: jailwick's).
  saveDefaultImage: (o: { base: string; user: string; home: string }) => call<unknown>('PUT', '/api/settings/image', o),
  pruneImages: () => call<{ removed: Image[] }>('POST', '/api/images/prune'),
  settings: () => call<Settings>('GET', '/api/settings'),
  about: () => call<About>('GET', '/api/about'),
  updateStatus: (refresh = false) => call<UpdateStatus>('GET', `/api/update${refresh ? '?refresh=1' : ''}`),
  runUpdate: () => call<UpdateRun>('POST', '/api/update'),
  saveUpdates: (on: boolean) => call<unknown>('PATCH', '/api/settings/updates', { on }),
}

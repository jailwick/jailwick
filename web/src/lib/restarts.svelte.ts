// Projects whose settings changed while they run (e.g. they joined a workspace): a restart applies
// it. Pages that change them add here; the restart banner offers the restarts.
export const restarts = $state<{ projects: string[] }>({ projects: [] })

export function needRestart(names: string[]) {
  restarts.projects = [...new Set([...restarts.projects, ...names])]
}

export function restarted(name: string) {
  restarts.projects = restarts.projects.filter((n) => n !== name)
}

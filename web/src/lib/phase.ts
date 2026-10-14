// What a project's sandbox is going through besides running or stopped: a start, restart or stop
// under way (its containers come and go: a restart recreates them), recovering (its main
// container crashed and the runtime brings it back), or paused (it waits for Resume). The page
// shows the phase, never running or stopped, while there is one.

import type { Health, Op } from './api'

export type Phase = '' | 'starting' | 'restarting' | 'stopping' | 'recovering' | 'paused'

export const phaseLabel: Record<Exclude<Phase, ''>, string> = {
  starting: 'Starting…',
  restarting: 'Restarting…',
  stopping: 'Stopping…',
  recovering: 'Recovering…',
  paused: 'Paused',
}

/** underWay: something is happening (a spinner); a paused sandbox waits for you instead. */
export const underWay = (p: Phase) => p !== '' && p !== 'paused'

// The page's own action (up, restart, down) until the server reports it.
const byAction: Record<string, Phase> = { up: 'starting', restart: 'restarting', down: 'stopping' }

/** phaseOf is the phase of a project: the page's own action first, then the op under way (the
 * app's or jw's), then what its main container is doing. A build without an op of its own is a
 * start's (a rebuild alone, op building, changes nothing). */
export function phaseOf(d: { op?: Op; build?: { failed?: boolean } | null; health?: Health } | undefined, action = ''): Phase {
  if (byAction[action]) return byAction[action]
  if (d?.op === 'starting' || d?.op === 'restarting' || d?.op === 'stopping') return d.op
  if (d?.build && !d.build.failed && d.op !== 'building') return 'starting'
  if (d?.health?.main === 'restarting') return 'recovering'
  if (d?.health?.main === 'paused') return 'paused'
  return ''
}

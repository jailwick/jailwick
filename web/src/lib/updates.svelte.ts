import { api, type UpdateStatus } from './api'

// Whether a newer jailwick is out: the sidebar marks About, which shows it. The server asks GitHub
// at most once a day (updates: off, only when asked); refresh checks now.
export const updates = $state<{ status: UpdateStatus | null }>({ status: null })

export async function loadUpdates(refresh = false): Promise<UpdateStatus> {
  const st = await api.updateStatus(refresh)
  updates.status = st
  return st
}

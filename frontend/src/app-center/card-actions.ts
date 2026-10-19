import type { AppCenterCard } from './schema.generated'
import { reasonLabel } from './labels.ts'

/** UI consumes permission facts; absent actions remain unavailable. */
export function managementActions(card: AppCenterCard, stale: boolean) {
  if (!card.installation || card.installation.state === 'removed') return []
  const kinds = card.actions.some(item => item.kind === 'upgrade') ? (['upgrade', 'stop', 'delete'] as const) : (['stop', 'delete'] as const)
  return kinds.map(kind => {
    const action = card.actions.find(item => item.kind === kind)
    return { kind, id: action?.id ?? null, enabled: !stale && action?.enabled === true,
      reason: stale ? '当前信息已过期，请刷新' : reasonLabel(action?.reasonCode ?? (action ? null : 'ACTION_UNSUPPORTED')) }
  })
}

export function versionLabel(card: AppCenterCard): string {
  return (card.installation ? card.installedVersion : card.candidateVersion) ?? '版本待确认'
}

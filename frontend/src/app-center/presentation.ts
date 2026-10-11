export function publisherLabel(id: string | null): string {
  const names: Record<string, string> = { vf: 'VerdantFlare', minimax: 'MiniMax', github: 'GitHub' }
  return id ? Object.hasOwn(names, id) ? names[id] : id : '发布方资料待补充'
}
export function observedTime(value: string | null): string {
  if (!value) return '尚无观测时间'
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : '时间待确认'
}
export function resourceLabel(kind: string): string {
  return ({ cpu: 'CPU', gpu: 'GPU', models: '模型' } as Record<string, string>)[kind] ?? kind
}

export function stationHealth(value: unknown, stationId: string): { name: string | null; status: '就绪' | '未就绪' | null } {
  if (!value || typeof value !== 'object') return { name: null, status: null }
  const data = value as Record<string, unknown>
  if (data.station_id !== stationId) return { name: null, status: null }
  const name = typeof data.cluster_name === 'string' && data.cluster_name.trim() ? data.cluster_name.trim() : null
  return { name, status: data.status === 'Ready' ? '就绪' : ['Degraded', 'Unavailable'].includes(String(data.status)) ? '未就绪' : null }
}

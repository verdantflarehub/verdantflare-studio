// Shared presentation of attributable telemetry. Zero is a valid sample.
export const validResourceNumber = value => typeof value === 'number' && Number.isFinite(value) && value >= 0;
export function formatResource(value, unit = 'cores') {
  if (!validResourceNumber(value)) return '—';
  if (unit === 'cores') return value.toLocaleString('zh-CN', {maximumFractionDigits: 3}) + ' 核';
  const index = value > 0 ? Math.min(4, Math.floor(Math.log(value) / Math.log(1024))) : 0;
  const scale = Math.max(0, index);
  return (value / 1024 ** scale).toLocaleString('zh-CN', {maximumFractionDigits: 2}) + ' ' + ['B','KiB','MiB','GiB','TiB'][scale];
}
export function resourceState(metric) {
  if (metric?.quality === 'stale') return 'stale';
  return metric?.quality === 'fresh' && validResourceNumber(metric.value) ? 'fresh' : 'unavailable';
}
export function resourceValue(metric, unit) {
  return resourceState(metric) === 'fresh' ? formatResource(metric.value, unit) : '—';
}

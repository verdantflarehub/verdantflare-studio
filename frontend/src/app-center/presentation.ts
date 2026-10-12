import { reasonLabels } from './labels.ts'

/** Observation explanations include successful states; they are not action denial codes. */
export function serviceReasonLabel(code: string | null): string {
  const observations: Record<string, string> = {
    replicas_ready: '服务副本已就绪', scaled_to_zero: '服务副本已停止', reconciling: '服务正在同步目标状态',
    scaling_down: '服务正在停止副本', deployment_failed: '服务部署异常', not_found: '未发现对应服务',
    forbidden: '暂无服务观测权限', unavailable: '暂时无法取得服务观测', invalid_response: '服务观测响应无效',
  }
  if (!code) return ''
  return Object.hasOwn(observations, code) ? observations[code] : Object.hasOwn(reasonLabels, code) ? reasonLabels[code] : '观测说明待确认'
}

export function publisherLabel(id: string | null): string {
  const names: Record<string, string> = { vf: 'VerdantFlare', minimax: 'MiniMax', github: 'GitHub' }
  return id ? Object.hasOwn(names, id) ? names[id] : id : '发布方资料待补充'
}
export function observedTime(value: string | null): string {
  if (!value) return '尚无观测时间'
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toLocaleString('zh-CN') : '时间待确认'
}
export function resourceLabel(kind: string): string {
  return ({ cpu: 'CPU', gpu: 'GPU', models: '模型' } as Record<string, string>)[kind] ?? kind
}

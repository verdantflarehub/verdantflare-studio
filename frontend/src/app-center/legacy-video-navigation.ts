/** Video's existing card title: project / shot key. No task schema extension. */
export function videoTaskName(task: unknown): string {
  if (!task || typeof task !== 'object') return '未命名任务'
  const value = task as Record<string, unknown>
  const key = typeof value.idempotency_key === 'string' ? value.idempotency_key.trim() : ''
  const project = typeof value.project_id === 'string' ? value.project_id.trim() : ''
  return key ? (project ? `${project} / ${key}` : key) : '未命名任务'
}

export function videoPage(route: string) {
  const path = new URL(route, 'https://studio.invalid').searchParams.get('video') ?? ''
  const task = /^\/dashboard\/tasks\/([a-zA-Z0-9_-]+)(?:#(?:tasks|models|mcp))?$/.exec(path)
  return { path, taskId: task?.[1] ?? '', section: task ? 'tasks' : path.split('#')[1] || 'tasks' }
}

export function videoTasksLocation(cardKey?: string): string {
  return '/market?video=' + encodeURIComponent('/dashboard#tasks') + (cardKey ? '&legacyCard=' + encodeURIComponent(cardKey) : '')
}

/** Only the authorized detail response for the currently displayed task names it. */
export function videoTaskLabel(route: string, request: string, status: number, task: unknown): string | null {
  const { taskId } = videoPage(route)
  if (!taskId || request !== `/api/tasks/${taskId}`) return null
  if (status === 404) return '未找到任务'
  if (status !== 200 || !task || typeof task !== 'object' || (task as Record<string, unknown>).video_task_id !== taskId) return '任务不可用'
  return videoTaskName(task)
}

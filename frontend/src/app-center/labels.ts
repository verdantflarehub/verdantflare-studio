export const actionLabels: Record<string, string> = { detail: '查看详情', refresh: '刷新', open: '打开', install: '安装', start: '启动', stop: '关闭', restart: '重启', delete: '删除', progress: '查看进度' }
export const statusLabels: Record<string, string> = {
  READY: '已就绪', STOPPED: '已安装 · 已关闭', STARTING: '启动中', STOPPING: '关闭中', DEGRADED: '服务异常', SERVICE_ABSENT: '服务未部署',
  NOT_INSTALLED: '未安装', INSTALLATION_PENDING: '安装待确认', INSTALLATION_FAILED: '安装失败', INSTALLATION_REMOVED: '已移除',
  STATE_UNKNOWN: '状态未知', BINDING_CONFLICT: '安装信息冲突', OBSERVED_DEPLOYMENT: '已有部署 · 待对账', OPERATION_IN_PROGRESS: '应用正在变更', RECOVERY_REQUIRED: '需要恢复',
}
export const reasonLabels: Record<string, string> = {
  ACTION_UNSUPPORTED: '此操作暂不可用', PERMISSION_DENIED: '当前账号没有权限', OPERATION_IN_PROGRESS: '请等待当前操作结束',
  BINDING_CONFLICT: '安装信息发生冲突，请刷新后检查', FACTS_UNAVAILABLE: '无法确认当前状态，请刷新',
  SERVICE_NOT_READY: '应用服务尚未就绪', ENTRY_UNAVAILABLE: '入口暂不可用', ENTRY_UNREGISTERED: '尚无可用入口',
  INSTALLATION_NOT_CONFIRMED: '安装尚未确认', INSTALLATION_STATE_CONFLICT: '安装状态已变化，请刷新',
  HOST_INCOMPATIBLE: '当前客户端不支持此入口', UNAUTHENTICATED: '请重新登录',
  APPCENTER_DEPENDENCY_UNAVAILABLE: '应用信息暂时无法加载，请稍后重试', APPCENTER_PROTOCOL_UNSUPPORTED: '服务正在更新，请稍后重试',
  APPCENTER_INVALID_VIEW: '应用信息暂时无法校验，请重试', APPCENTER_INVALID_FACTS: '应用信息暂时无法校验，请重试',
}
export const reasonLabel = (code: string | null) => code ? reasonLabels[code] ?? '当前暂不可用，请稍后重试' : ''
const factLabels: Record<string, string> = {
  pending: '待确认', installed: '已安装', failed: '失败', removed: '已移除', ready: '已就绪', stopped: '已关闭', starting: '启动中', stopping: '关闭中', degraded: '服务异常', absent: '未部署', unknown: '未知',
  available: '可用', unavailable: '暂不可用', denied: '无访问权限', incompatible: '客户端不兼容', accepted: '已受理', running: '进行中', succeeded: '已完成', completed: '已结束', recovery_required: '需要恢复',
  checking: '检查中', downloading: '下载中', verifying: '校验中', installing: '安装中', warming: '准备中', deleting: '删除中', sealing: '保护当前工作', applying: '应用更新', publishing: '确认版本', releasing: '恢复服务', restoring: '恢复原版本',
}
export const factLabel = (code: string | null | undefined) => code ? factLabels[code] ?? '状态待确认' : '暂无'

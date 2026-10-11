export const actionLabels: Record<string, string> = { detail: '查看详情', refresh: '刷新', open: '打开', install: '安装', start: '启动', stop: '关闭', restart: '重启', delete: '删除', progress: '查看进度' }
export const statusLabels: Record<string, string> = {
  READY: '已就绪', STOPPED: '已安装 · 已关闭', STARTING: '启动中', STOPPING: '关闭中', DEGRADED: '服务异常', SERVICE_ABSENT: '服务未部署',
  NOT_INSTALLED: '未安装', INSTALLATION_PENDING: '安装待确认', INSTALLATION_FAILED: '安装失败', INSTALLATION_REMOVED: '已移除',
  STATE_UNKNOWN: '状态未知', BINDING_CONFLICT: '安装信息冲突', OBSERVED_DEPLOYMENT: '已有部署 · 待对账', OPERATION_IN_PROGRESS: '应用正在变更', RECOVERY_REQUIRED: '需要恢复',
}
export const reasonLabels: Record<string, string> = {
  APPCENTER_LEGACY_RECOVERY_INVALID: '旧操作记录无法校验，原记录已保留。请检查原操作后恢复。',
  APPCENTER_LEGACY_STORAGE_UNAVAILABLE: '暂时无法读取旧操作记录，新提交已暂停。',
  APPCENTER_LEGACY_CONFLICT: '旧操作记录发生冲突，原记录已保留，请刷新后检查。',
  APPCENTER_OPERATION_UNAVAILABLE: '暂时无法读取操作进度，请稍后刷新。',
  APPCENTER_INVALID_OPERATION: '操作响应与当前应用不一致，请刷新后检查。',
  SERVICE_UNAVAILABLE: '服务暂时不可用，请检查应用状态后重试。', INTERNAL: '操作发生错误，请检查应用状态。',
  APPCENTER_RECOVERY_STORAGE_UNAVAILABLE: '恢复存储不可用，新的提交已暂停。请检查存储并确认已有操作状态。',
  APPCENTER_RECOVERY_SCOPE_CHANGED: '登录状态已变化，请重新登录后检查原操作。',
  APPCENTER_RECOVERY_LIMIT: '待确认操作过多，请先恢复已有操作。',
  APPCENTER_RECOVERY_PENDING: '此应用已有待确认请求，请先恢复原请求。',
  APPCENTER_SUBMISSION_UNKNOWN: '尚无法确认提交结果，请恢复原请求。',
  CONFLICT: '请求与已有操作冲突，请检查原操作和应用状态。', RELEASE_UNVERIFIED: '此版本尚未完成发行校验。',
  INVALID_LEGACY_ROUTE: '旧入口地址无效，请从应用中心重新打开', LEGACY_ENTRY_AMBIGUOUS: '无法唯一确定此入口对应的应用，请从卡片打开',
  LEGACY_BACKEND_CHANGED: '应用部署或入口配置已变化，请刷新后重试', APPCENTER_INVALID_OPEN: '应用入口响应无法校验，请重试',
  LEGACY_ENTRY_UNMAPPED: '此旧入口尚未完成对象对账，请返回应用中心查看当前入口',
  ENTRY_UNVERIFIED: '入口资料尚未完成校验', HOST_UNSUPPORTED: '当前客户端不支持此入口',
  APPCENTER_BINDING_CHANGED: '应用状态或入口已变化，请刷新后重新打开', APPCENTER_OPEN_UNAVAILABLE: '此应用当前不可打开',
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

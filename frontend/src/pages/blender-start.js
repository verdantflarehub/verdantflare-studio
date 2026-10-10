import { canStart, canStop, canDestroy, instanceName } from './blender-management.js';

const validID = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[47][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value);

// Persist before sending: refresh or uncertain responses replay the same command.
export function createBlenderStarter({ request, storage, newID = () => crypto.randomUUID(), action = 'start' }) {
  if (!['start','stop','destroy'].includes(action)) throw Error('不支持的实例操作。');
  const allowed = {start:canStart,stop:canStop,destroy:canDestroy}[action], label = {start:'启动',stop:'停止',destroy:'销毁'}[action];
  let busy = false;
  return async (item, confirmation) => {
    if (busy || !allowed(item)) return null;
    if(action === 'destroy' && confirmation !== instanceName(item)) throw Error('请输入完整的实例名称以确认销毁。');
    busy = true;
    let key;
    try {
      const actor = await request('/studio/apps/blender/instance-options');
      if (actor.can_manage !== true || ![actor.station_id,actor.organization_id,actor.user_id,item.instance_id].every(validID)
          || !/^blender[A-Za-z0-9_-]{1,57}$/.test(item.alias)) throw Error('当前账号或实例不允许'+label+'。');
      key = 'vf-blender-'+action+':' + [actor.station_id,actor.organization_id,actor.user_id,item.instance_id].join(':');
      let draft;
      const raw = storage.getItem(key);
      if (raw !== null) {
        try { draft = JSON.parse(raw); } catch { /* Fail closed below. */ }
        if (!draft || draft.alias !== item.alias || draft.instance_id !== item.instance_id || !validID(draft.body?.idempotency_key)
            || !Number.isSafeInteger(draft.body?.expected_version) || draft.body.expected_version < 1
            || Object.keys(draft.body).sort().join(',') !== (action === 'destroy' ? 'confirm_name,expected_version,idempotency_key' : 'expected_version,idempotency_key')
            || (action === 'destroy' && draft.body.confirm_name !== confirmation)) {
          throw Error(label+'恢复记录无法读取，请先核对实例状态。');
        }
      } else {
        draft = {alias:item.alias,instance_id:item.instance_id,body:{expected_version:item.state_version,idempotency_key:newID()}};
        if(action === 'destroy')draft.body.confirm_name=confirmation;
        storage.setItem(key,JSON.stringify(draft));
      }
      const result = await request('/studio/apps/blender/instances/'+encodeURIComponent(draft.alias)+'/'+action,draft.body);
      if (result.instance_id !== item.instance_id || result.alias !== item.alias || result.action !== action
          || !validID(result.operation_id) || !['running','completed','failed'].includes(result.state)) {
        throw Error(label+'结果尚未确认，请刷新并重试原请求。');
      }
      // Once acknowledged, the directory retains operation progress across reloads.
      // A storage failure preserves the original key rather than creating a new start.
      storage.removeItem(key);
      return result;
    } catch (error) {
      // The server returns these only before accepting this idempotency key.
      if (key && ['INSTANCE_STATE_CONFLICT','INSTANCE_BUSY'].includes(error.code)) storage.removeItem(key);
      throw error;
    } finally { busy = false; }
  };
}

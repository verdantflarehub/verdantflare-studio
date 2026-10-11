import { isMockEnabled, handleMockRequest } from '../mocks'

export interface Request {path:string;method:string;body?:unknown}
interface Result {status:number;data:unknown;request_id:string}
const desktop = new URLSearchParams(location.search).get('host') === 'desktop'

export async function request(input:Request, options: {realOnly?: boolean; signal?: AbortSignal} = {}):Promise<Response> {
 options.signal?.throwIfAborted()
 if (!options.realOnly && isMockEnabled()) {
  const mockRes = await handleMockRequest(input)
  if (mockRes) return mockRes
 }
 if (desktop) {
  const { Call } = await import('@wailsio/runtime')
  options.signal?.throwIfAborted()
  const pending = Call.ByName('github.com/verdantflarehub/verdantflare-studio/internal/transport/desktop.Service.Call',input)
  const cancel = () => { void pending.cancel().catch(() => undefined) }
  options.signal?.addEventListener('abort', cancel, { once: true })
  let result: Result
  try { result = await pending as Result; options.signal?.throwIfAborted() }
  finally { options.signal?.removeEventListener('abort', cancel) }
  return new Response(result.status===204?null:JSON.stringify(result.data),{status:result.status,headers:{'Content-Type':'application/json','X-Request-ID':result.request_id}})
 }
 return fetch('/api/'+input.path,{method:input.method,credentials:'same-origin',cache:'no-store',headers:input.body?{'Content-Type':'application/json'}:{},body:input.body?JSON.stringify(input.body):undefined,signal:options.signal})
}

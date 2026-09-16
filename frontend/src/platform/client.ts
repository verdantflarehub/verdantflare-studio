export interface Request {path:string;method:string;body?:unknown}
interface Result {status:number;data:unknown;request_id:string}
const desktop = new URLSearchParams(location.search).get('host') === 'desktop'
export async function request(input:Request):Promise<Response> {
 if (desktop) {
  const { Call } = await import('@wailsio/runtime')
  const result = await Call.ByName('github.com/verdantflarehub/verdantflare-studio/internal/transport/desktop.Service.Call',input) as Result
  return new Response(result.status===204?null:JSON.stringify(result.data),{status:result.status,headers:{'Content-Type':'application/json','X-Request-ID':result.request_id}})
 }
 return fetch('/studio/api/'+input.path,{method:input.method,credentials:'same-origin',cache:'no-store',headers:input.body?{'Content-Type':'application/json'}:{},body:input.body?JSON.stringify(input.body):undefined})
}

import test from 'node:test'
import assert from 'node:assert/strict'
import { createFileAccess, pickJSONFiles } from '../src/app-dashboard/files.ts'

const signal = () => new AbortController().signal
const purpose = 'json.import'
function fixture(t: { after(fn: () => void): void }) {
  let clock = Date.now(), allowed = true
  let selected = [new File(['{"nodes":[],"links":[]}'], 'workflow.json')]
  const grants: string[] = []
  const access = createFileAccess({expiresAt:new Date(clock+900_000).toISOString(), now:()=>clock,
    async authorize(capability) {grants.push(capability);if(!allowed)throw Error('CAPABILITY_DENIED')},
    pick:async()=>selected, read:file=>file.arrayBuffer()})
  t.after(access.clear)
  return {access, grants, select:()=>access.select({purpose},signal()), deny:()=>{allowed=false},
    advance:(ms:number)=>{clock+=ms}, files:(value:File[])=>{selected=value}}
}

test('file handle is scoped, consumed once and reveals no native object or path', async t => {
  const f=fixture(t), other=fixture(t)
  f.files([new File(['{}'],'private/directory/workflow.json')])
  const [file]=await f.select()
  assert.deepEqual(Object.keys(file!).sort(),['handle','mime','name','size'])
  assert.equal(file!.name,'workflow.json')
  assert.equal(file!.mime,'application/json')
  await assert.rejects(other.access.read({handle:file!.handle,purpose},signal()),/FILE_HANDLE_EXPIRED/)
  assert.deepEqual(await f.access.read({handle:file!.handle,purpose},signal()),{text:'{}'})
  assert.deepEqual(f.grants,['files.select','files.read','files.read'])
  await assert.rejects(f.access.read({handle:file!.handle,purpose},signal()),/FILE_HANDLE_EXPIRED/)
})

test('denied selection, expired handles, clear and changed authorization prevent content access', async t => {
  const f=fixture(t)
  let [file]=await f.select()
  f.advance(300_000)
  await assert.rejects(f.access.read({handle:file!.handle,purpose},signal()),/FILE_HANDLE_EXPIRED/)
  ;[file]=await f.select(); f.access.clear()
  await assert.rejects(f.access.read({handle:file!.handle,purpose},signal()),/FILE_HANDLE_EXPIRED/)
  ;[file]=await f.select(); f.deny()
  await assert.rejects(f.access.read({handle:file!.handle,purpose},signal()),/CAPABILITY_DENIED/)
  await assert.rejects(f.select(),/CAPABILITY_DENIED/)
})

test('limits and purpose are enforced before issuing handles; cancelled selection is empty', async t => {
  const f=fixture(t)
  assert.throws(()=>f.access.select({purpose:'anything'},signal()),/FILE_PURPOSE_UNSUPPORTED/)
  for (const files of [[new File(['{}'],'x.exe')],[new File([new Uint8Array(1024*1024+1)],'x.json')],
    [new File(['{}'],'x.json'),new File(['{}'],'y.json')],[new File(['{}'],'bad\n.json')]]) {
    f.files(files);await assert.rejects(f.select())
  }
  f.files([]);assert.deepEqual(await f.select(),[])
  f.files(Array.from({length:8},(_,i)=>new File(['{}'],`${i}.json`)))
  await f.access.select({purpose,multiple:true},signal());await f.access.select({purpose,multiple:true},signal())
  await assert.rejects(f.access.select({purpose,multiple:true},signal()),/FILE_HANDLE_LIMIT/)
})

test('revocation during read discards bytes and invalid UTF-8 is rejected', async t => {
  let reject = false
  const access=createFileAccess({expiresAt:new Date(Date.now()+60_000).toISOString(),
    authorize:async()=>{if(reject)throw Error('CAPABILITY_DENIED')},
    pick:async()=>[new File(['{}'],'x.json')],read:async file=>{reject=true;return file.arrayBuffer()}})
  t.after(access.clear)
  const [file]=await access.select({purpose},signal())
  await assert.rejects(access.read({handle:file!.handle,purpose},signal()),/CAPABILITY_DENIED/)
  const f=fixture(t);f.files([new File([new Uint8Array([255])],'x.json')])
  const [invalid]=await f.select()
  await assert.rejects(f.access.read({handle:invalid!.handle,purpose},signal()),/INVALID_FILE_ENCODING/)
})

test('late picker after abort or scope clear cannot issue a handle', async t => {
  let finish!: (files: File[]) => void
  const access=createFileAccess({expiresAt:new Date(Date.now()+60_000).toISOString(),authorize:async()=>{},
    pick:()=>new Promise(resolve=>{finish=resolve})})
  t.after(access.clear)
  for(const cancel of ['abort','clear']) {
    const controller=new AbortController(), operation=access.select({purpose},controller.signal)
    const rejected=assert.rejects(operation,/REQUEST_CANCELLED/)
    if(cancel==='abort')controller.abort();else access.clear()
    finish([new File(['{}'],'x.json')]);await rejected
  }
})

test('host picker opens synchronously, cleans up on cancel, and serializes dialogs', async t => {
  const original=Object.getOwnPropertyDescriptor(globalThis,'document')
  const inputs: (EventTarget & {files?:File[],showPicker():void,remove():void,removed?:boolean})[]=[]
  let opened=0
  Object.defineProperty(globalThis,'document',{configurable:true,value:{body:{append(){}},createElement(){
    const input=Object.assign(new EventTarget(),{showPicker(){opened++},remove(){this.removed=true},removed:false})
    inputs.push(input);return input
  }}})
  t.after(()=>{if(original)Object.defineProperty(globalThis,'document',original);else Reflect.deleteProperty(globalThis,'document')})
  const controller=new AbortController(), first=pickJSONFiles({purpose},controller.signal)
  assert.equal(opened,1)
  await assert.rejects(pickJSONFiles({purpose},signal()),/FILE_PICKER_BUSY/)
  inputs[0]!.dispatchEvent(new Event('cancel'));assert.deepEqual(await first,[]);assert.equal(inputs[0]!.removed,true)
  const next=pickJSONFiles({purpose},controller.signal), rejected=assert.rejects(next,/REQUEST_CANCELLED/)
  controller.abort();await rejected;assert.equal(inputs[1]!.removed,true)
})

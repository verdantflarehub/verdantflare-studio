import assert from 'node:assert/strict'
import test from 'node:test'
import { readFile } from 'node:fs/promises'
import { createRenderer, h, markRaw, nextTick, ref } from 'vue'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

// Exercise the actual SFC with Vue's renderer, without a browser or service.
async function component(name) {
  const file = new URL(`../src/app-center/${name}.vue`, import.meta.url)
  const { descriptor } = parse(await readFile(file, 'utf8'), { filename: file.pathname })
  let source = compileScript(descriptor, { id: name, inlineTemplate: true }).content
  source = source.replace(/import AppIcon from '\.\/AppIcon.vue'/, 'const AppIcon = () => null')
  source = source.replace(/from (['"])([^'"]+)\1/g, (match, _quote, path) => {
    if (path === 'vue') return `from '${import.meta.resolve('vue')}'`
    if (path.startsWith('./')) return `from '${new URL(`${path.replace(/\.ts$/, '')}.ts`, file).href}'`
    return match
  })
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
  return (await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)).default
}

class Element {
  constructor(tag) { this.tag = tag; this.children = []; this.props = {}; this.parent = null; markRaw(this) }
  contains(node) { return node === this || this.children.some(child => child.contains(node)) }
  querySelectorAll() {
    return this.children.flatMap(child => [...(child.props.role === 'menuitem' ? [child] : []), ...child.querySelectorAll()])
  }
  focus() { document.activeElement = this; for (const listener of listeners.get('focusin') ?? []) listener({ target: this }) }
}
const listeners = new Map()
const renderer = createRenderer({
  createElement: tag => new Element(tag), createText: text => Object.assign(new Element('#text'), { text }),
  createComment: text => Object.assign(new Element('#comment'), { text }),
  insert(node, parent, anchor = null) {
    if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1)
    const index = anchor ? parent.children.indexOf(anchor) : -1
    if (index < 0) parent.children.push(node); else parent.children.splice(index, 0, node)
    node.parent = parent
  },
  remove(node) { node.parent?.children.splice(node.parent.children.indexOf(node), 1); node.parent = null },
  setText(node, text) { node.text = text }, setElementText(node, text) { node.text = text; node.children = [] },
  parentNode: node => node.parent, nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
  patchProp(node, key, _old, value) { node.props[key] = value },
})
const find = (root, predicate) => predicate(root) ? root : root.children.map(child => find(child, predicate)).find(Boolean)
const action = (id, kind, enabled = true) => ({ id, kind, enabled, reasonCode: enabled ? null : 'PERMISSION_DENIED', entryId: null, confirmation: 'none' })

test('card menu dispatches exact server actions, supports focus navigation, denies stale actions and cleans up', async () => {
  const originalDocument = globalThis.document
  globalThis.document = { activeElement: null,
    addEventListener(type, fn) { if (!listeners.has(type)) listeners.set(type, new Set()); listeners.get(type).add(fn) },
    removeEventListener(type, fn) { listeners.get(type)?.delete(fn) },
  }
  const AppCard = await component('AppCard')
  const card = ref({ cardKey: 'installation:example', display: { name: 'Example', publisherId: null, iconRef: null, description: '', resourceTags: [] },
    installation: { state: 'installed' }, installedVersion: '1.0.0', candidateVersion: '2.0.0', primaryActionId: 'entry.main', statusCode: 'READY',
    actions: [action('entry.main', 'open'), action('service.stop', 'stop'), action('installation.remove', 'delete', false)] })
  const stale = ref(false), menuOpen = ref(false), emitted = []
  const root = new Element('root')
  const app = renderer.createApp({ setup: () => () => h(AppCard, { card: card.value, stale: stale.value, menuOpen: menuOpen.value,
    onMenu: key => { menuOpen.value = key === card.value.cardKey }, onAction: (target, id) => emitted.push([target.cardKey, id]) }) })
  try {
    app.mount(root)
    const trigger = () => find(root, node => node.props['aria-haspopup'] === 'menu')
    const menu = () => find(root, node => node.props.role === 'menu')
    await trigger().props.onClick(); await nextTick()
    assert.equal(menuOpen.value, true)
    assert.equal(document.activeElement, menu().querySelectorAll()[0])
    const key = value => ({ key: value, preventDefault() {}, stopPropagation() {} })
    menu().props.onKeydown(key('ArrowDown'))
    assert.equal(document.activeElement, menu().querySelectorAll()[1])
    menu().querySelectorAll()[1].props.onClick()
    assert.deepEqual(emitted, [], 'disabled remove never emits a command')
    menu().props.onKeydown(key('Escape')); await nextTick()
    assert.equal(menuOpen.value, false); assert.equal(document.activeElement, trigger())
    await trigger().props.onClick(); await nextTick()
    menu().querySelectorAll()[0].props.onClick(); await nextTick()
    assert.deepEqual(emitted, [['installation:example', 'service.stop']])
    assert.equal(menuOpen.value, false)
    assert.equal(document.activeElement, trigger())
    stale.value = true; await nextTick()
    await trigger().props.onClick(); await nextTick()
    assert.ok(menu().querySelectorAll().every(node => node.props['aria-disabled'] === true))
    menu().querySelectorAll()[0].props.onClick()
    assert.equal(emitted.length, 1)
    for (const listener of listeners.get('pointerdown')) listener({ target: new Element('outside') })
    await nextTick(); assert.equal(menuOpen.value, false)
    stale.value = false; card.value.primaryActionId = 'service.start'; card.value.actions.push(action('service.start', 'start'))
    await nextTick()
    find(root, node => node.tag === 'button' && node.text === '启动').props.onClick()
    assert.deepEqual(emitted[1], ['installation:example', 'service.start'])
  } finally {
    app.unmount()
    assert.ok([...listeners.values()].every(set => set.size === 0))
    globalThis.document = originalDocument
  }
})

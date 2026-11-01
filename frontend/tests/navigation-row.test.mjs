import assert from 'node:assert/strict'
import test from 'node:test'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import { createRenderer } from 'vue'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

const require = createRequire(import.meta.url)
const vueUrl = pathToFileURL(require.resolve('vue')).href
async function loadComponent(name, imports = {}) {
  const source = await readFile(new URL(`../src/components/${name}.vue`, import.meta.url), 'utf8')
  const { descriptor } = parse(source)
  const compiled = compileScript(descriptor, { id: name, inlineTemplate: true })
  let code = ts.transpileModule(compiled.content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText
  code = code.replace(/from ['"]vue['"]/g, `from '${vueUrl}'`)
  for (const [path, url] of Object.entries(imports)) code = code.replace(`from '${path}'`, `from '${url}'`)
  return 'data:text/javascript;base64,' + Buffer.from(code).toString('base64')
}
const iconUrl = await loadComponent('NavigationIcon')
const { default: NavigationRow } = await import(await loadComponent('NavigationRow', { './NavigationIcon.vue': iconUrl }))

// In-memory Vue renderer: exercises the real SFC without a browser or development server.
const renderer = createRenderer({
  createElement: type => ({ type, props: {}, children: [] }),
  createText: text => ({ type: '#text', text }), createComment: text => ({ type: '#comment', text }),
  setText: (node, text) => { node.text = text }, setElementText: (node, text) => { node.text = text },
  patchProp: (node, key, _old, value) => { node.props[key] = value },
  insert: (node, parent) => { node.parent = parent; parent.children.push(node) },
  remove: node => { node.parent.children.splice(node.parent.children.indexOf(node), 1) },
  parentNode: node => node.parent, nextSibling: () => null,
})
function mount(props) {
  const root = { children: [] }
  const app = renderer.createApp(NavigationRow, props)
  app.mount(root)
  return { row: root.children[0], close: () => app.unmount() }
}
function click(node, extra = {}) {
  let prevented = false
  node.props.onClick({ button: 0, preventDefault() { prevented = true }, ...extra })
  return prevented
}
test('parent navigation and disclosure share one row while preserving independent actions and focus targets', () => {
  let opens = 0, toggles = 0
  const mounted = mount({ label: '任意名称', icon: 'globe', href: '/apps/example/overview', children: true, expanded: true, controls: 'example-children', current: true,
    onActivate: () => opens++, onToggle: () => toggles++ })
  const actions = mounted.row.children.filter(node => ['a', 'button'].includes(node.type))
  assert.equal(actions.length, 2)
  assert.match(mounted.row.props.class, /menu-row.*current/)
  assert.equal(actions[0].props.href, '/apps/example/overview')
  assert.equal(actions[0].props['aria-current'], 'page')
  assert.equal(actions[1].props['aria-expanded'], true)
  assert.equal(actions[1].props['aria-controls'], 'example-children')
  assert.equal(actions[1].props['aria-label'], '收起任意名称')
  click(actions[0]); click(actions[1])
  assert.deepEqual([opens, toggles], [1, 1])
  assert.equal(click(actions[0], { ctrlKey: true }), false)
  assert.deepEqual([opens, toggles], [1, 1])
  mounted.close()
})
test('container and leaf nodes use the same row with appropriate native controls', () => {
  let toggles = 0
  const container = mount({ label: '分类', children: true, expanded: false, controls: 'items', onToggle: () => toggles++ })
  const button = container.row.children.find(node => node.type === 'button')
  assert.equal(button.props['aria-expanded'], false)
  assert.equal(button.props['aria-controls'], 'items')
  click(button)
  assert.equal(toggles, 1)
  assert.equal(container.row.children.filter(node => node.type === 'button').length, 1)
  const leaf = mount({ label: '服务端标题', href: '/apps/example/items', child: true, current: true })
  assert.match(leaf.row.props.class, /menu-row.*child.*current/)
  assert.equal(leaf.row.children.some(node => node.type === 'button'), false)
  const link = leaf.row.children.find(node => node.type === 'a')
  assert.equal(link.props['aria-expanded'], undefined)
  assert.equal(link.props['aria-label'], '服务端标题')
  container.close(); leaf.close()
})

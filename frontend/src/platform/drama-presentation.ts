import type { DramaGlobal, DramaVersion, ProjectContent } from './project-content'

/** Viewing a candidate never changes selections or approval. Prefer a selected visual,
 * otherwise use the latest registered visual candidate in the supplied order. */
export function materialPreview(material: DramaGlobal) {
  const image = (v: DramaVersion) => v.file_refs.find(f => ['image', 'preview', 'portrait'].includes(f.purpose))
  const selected = material.versions.find(v => material.selected_versions.some(s => s.version_key === v.key) && image(v))
  const version = selected || [...material.versions].reverse().find(v => image(v))
  return { version, fileId: version ? image(version)!.file_id : (!material.versions.length && !['sound', 'music'].includes(material.kind) ? material.file_ids[0] : undefined), selected: !!selected }
}

/** Display aliases only; the stored text, keys and fixed references stay intact. */
export function dramaText(text: string | null | undefined, drama?: ProjectContent['ai_short_drama']): string {
  let result = text || ''
  if (drama) {
    const names = new Map(drama.globals.map(g => [g.key, g.name]))
    const sceneNames = new Map<string, Set<string>>()
    for (const ep of drama.episodes) for (const scene of ep.scenes) {
      names.set(scene.key, scene.title)
      const sharedKey = /^(S\d+)_SH\d+$/.exec(scene.key)?.[1]
      if (sharedKey) { const values = sceneNames.get(sharedKey) || new Set<string>(); values.add(scene.title); sceneNames.set(sharedKey, values) }
      for (const shot of scene.shots) names.set(shot.key, `「${shot.title}」`)
    }
    for (const [key, values] of sceneNames) if (values.size === 1) names.set(key, [...values][0])
    const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const keys = [...names.keys()].sort((a, b) => b.length - a.length)
    if (keys.length) result = result.replace(new RegExp(`(?<![A-Za-z0-9_])(?:${keys.map(escape).join('|')})(?![A-Za-z0-9_])`, 'g'), key => names.get(key)!)
  }
  // Render links as their authored labels. Local source paths and Markdown markup
  // are not interactive URLs in the customer reader.
  return result.replace(/!?\[([^\]]+)\]\([^\n)]*\)/g, '$1').replace(/`([^`]+)`/g, '$1')
}

/** Resolve only the requested authored section; missing anchors must not silently
 * display a different state or the entire document as that state's basis. */
export function documentSection(text: string, anchor?: string): string {
  if (!anchor) return text
  const target = decodeURIComponent(anchor.replace(/^#/, ''))
  const lines = text.split('\n')
  const slug = (heading: string) => heading.toLocaleLowerCase().replace(/<[^>]+>/g, '').replace(/[^\p{L}\p{N}_ -]/gu, '').trim().replace(/ /g, '-')
  let start = lines.findIndex(line => {
    const explicit = /^\s*<a\s+(?:id|name)=["']([^"']+)["']\s*>\s*<\/a>\s*$/.exec(line)
    return explicit?.[1] === target || /^#{1,6}\s+/.test(line) && slug(line.replace(/^#{1,6}\s+/, '')) === target
  })
  if (start < 0) throw new Error('当前固定资料没有对应章节，请核对来源引用。')
  let heading = start
  while (heading >= 0 && !/^#{1,6}\s+/.test(lines[heading])) heading--
  if (heading >= 0) start = heading
  const level = /^#+/.exec(lines[start])?.[0].length || 6
  let end = start + 1
  while (end < lines.length && !new RegExp(`^#{1,${level}}\\s+`).test(lines[end])) end++
  return lines.slice(start, end).filter(line => !/^\s*<a\s+(?:id|name)=["'][^"']+["']\s*>\s*<\/a>\s*$/.test(line)).join('\n').trim()
}

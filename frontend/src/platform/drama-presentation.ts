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
    for (const ep of drama.episodes) for (const scene of ep.scenes) for (const shot of scene.shots) names.set(shot.key, `「${shot.title}」`)
    const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const keys = [...names.keys()].sort((a, b) => b.length - a.length)
    if (keys.length) result = result.replace(new RegExp(`(?<![A-Za-z0-9_])(?:${keys.map(escape).join('|')})(?![A-Za-z0-9_])`, 'g'), key => names.get(key)!)
  }
  // Render links as their authored labels. Local source paths and Markdown markup
  // are not interactive URLs in the customer reader.
  return result.replace(/!?\[([^\]]+)\]\([^\n)]*\)/g, '$1').replace(/`([^`]+)`/g, '$1')
}

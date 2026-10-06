const projectA = '0192f3d4-3111-7aaa-8bbb-1234567890ab'
const projectB = '0192f3d4-3222-7aaa-8bbb-1234567890ab'
export const mockCreatedProjectID = '0192f3d4-3333-7aaa-8bbb-1234567890ab'
const revisionA = '0192f3d4-4111-7aaa-8bbb-1234567890ab'
const revisionB = '0192f3d4-4222-7aaa-8bbb-1234567890ab'
const assetVoice = '0192f3d4-5111-7aaa-8bbb-1234567890ab'
const assetCharacter = '0192f3d4-5222-7aaa-8bbb-1234567890ab'
const assetVersionVoice = '0192f3d4-6111-7aaa-8bbb-1234567890ab'
const assetVersionCharacter = '0192f3d4-6222-7aaa-8bbb-1234567890ab'
export const mockSavedRevision = '0192f3d4-7444-7aaa-8bbb-1234567890ab'

export const mockProjects = [
  { project_id: projectA, head_revision_id: revisionA, name: '《空心》声音制作', category: 'music', status: 'active', created_at: '2026-10-01T08:00:00Z' },
  { project_id: projectB, head_revision_id: revisionB, name: '小月舞蹈镜头', category: 'video', status: 'draft', created_at: '2026-10-04T08:00:00Z' }
]

export const mockAssets = [
  { asset_id: assetVoice, head_asset_version_id: assetVersionVoice, name: 'Mengsk 声音模型', asset_type: 'voice-model', subjects: ['Mengsk'], created_at: '2026-10-01T08:00:00Z' },
  { asset_id: assetCharacter, head_asset_version_id: assetVersionCharacter, name: '小月人物形象', asset_type: 'character-image', subjects: ['小月'], created_at: '2026-10-04T08:00:00Z' }
]

export const mockEntryTexts = new Map<string, string>([
  [projectA, '# 制作审核记录\n\n## 《空心》\n\n入口文档来自服务端 Project 修订。'],
  [projectB, '# 小月舞蹈镜头\n\n## 当前目标\n\n使用固定人物形象版本。']
])

export function mockProjectOpen(projectID: string) {
  const project = mockProjects.find(item => item.project_id === projectID) || mockProjects[0]
  const revision = project.head_revision_id
  const fileID = '0192f3d4-7111-7aaa-8bbb-1234567890ab'
  return {
    project_id: project.project_id,
    revision_id: revision,
    head_revision_id: revision,
    manifest_ref: { store_id: '0192f3d4-8111-7aaa-8bbb-1234567890ab', artifact_id: '0192f3d4-8222-7aaa-8bbb-1234567890ab', version_id: '0192f3d4-8333-7aaa-8bbb-1234567890ab' },
    manifest: {
      schema_version: 1,
      kind: 'project',
      project_id: project.project_id,
      name: project.name,
      category: project.category,
      status: project.status,
      entry_document_id: fileID,
      files: [{ file_id: fileID, path: 'review.md', role: 'review', content_ref: { store_id: '0192f3d4-9111-7aaa-8bbb-1234567890ab', artifact_id: '0192f3d4-9222-7aaa-8bbb-1234567890ab', version_id: '0192f3d4-9333-7aaa-8bbb-1234567890ab' } }],
      selections: [],
      asset_refs: project.category === 'video' ? [{ asset_id: assetCharacter, asset_version_id: assetVersionCharacter, purpose: 'character-reference' }] : [],
      run_refs: [],
      domain_documents: []
    },
    invalidated_selections: []
  }
}

export function mockAssetGet(assetID: string, versionID: string) {
  const asset = mockAssets.find(item => item.asset_id === assetID) || mockAssets[0]
  return {
    asset_id: asset.asset_id,
    asset_version_id: versionID,
    manifest_ref: { store_id: '0192f3d4-a111-7aaa-8bbb-1234567890ab', artifact_id: '0192f3d4-a222-7aaa-8bbb-1234567890ab', version_id: '0192f3d4-a333-7aaa-8bbb-1234567890ab' },
    manifest: {
      schema_version: 1,
      kind: 'asset-version',
      asset_id: asset.asset_id,
      name: asset.name,
      asset_type: asset.asset_type,
      subjects: asset.subjects,
      files: [{ file_id: '0192f3d4-b111-7aaa-8bbb-1234567890ab', path: asset.asset_type === 'voice-model' ? 'voice-model.json' : 'identity.md', role: 'domain', content_ref: { store_id: '0192f3d4-b222-7aaa-8bbb-1234567890ab', artifact_id: '0192f3d4-b333-7aaa-8bbb-1234567890ab', version_id: '0192f3d4-b444-7aaa-8bbb-1234567890ab' } }],
      source: { project_id: projectA, project_revision_id: revisionA, relation: 'produced_in' },
      depends_on: []
    }
  }
}

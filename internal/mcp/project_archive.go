package mcp

// ProjectArchiveInstructions applies to project creation tasks, not read-only calls.
const ProjectArchiveInstructions = `For AI short-drama creation or revision tasks, save each completed stage to the same Studio Project (category ai_short_drama), unless the user explicitly requests local-only work. Open the project first and preserve its real project_id across script, Image, Video, Music and Blender work. Archive script packages, settings, candidates, revisions, lighting/time/weather states, Blender projects, shots and deliverables as applicable. Preserve stable object IDs, state/version distinctions, change notes, generation records and fixed references in the project-content domain document. Use discovered Project/Artifact schemas, controlled uploads and expected_revision_id; a native media artifact ID is not a central ContentRef. If a service already committed the output, read that revision instead of duplicating it. Verify the saved revision and content before reporting synchronization. Recover unknown writes with the original IDs and commit_status; never regenerate paid content merely because synchronization failed. Read-only discussion causes no writes. Saving candidates does not approve, select or publish them to World; World publication requires an explicitly chosen version and its source project/revision.`

const projectArchiveToolHint = "AI short-drama stage outputs must be saved and verified in the same Studio Project unless explicitly local-only. Keep fixed versions, states, change history and source references; preserve candidate approval status. Follow the host initialization archive instructions."

func archiveGuidanceDomain(domain string) bool {
	switch domain {
	case "project", "artifact", "world", "image", "video", "music", "blender":
		return true
	default:
		return false
	}
}

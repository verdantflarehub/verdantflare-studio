package dashboard

// RegisteredArtifacts returns only locally registered immutable artifact
// identities. Reading these does not contact an App or assert that it is online.
func (s *RegisteredSource) RegisteredArtifacts() (map[string]string, error) {
	registry, _, err := s.registry()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(registry.Releases))
	for _, release := range registry.Releases {
		out[release.AppID+"@"+release.AppVersion] = release.ArtifactDigest
	}
	return out, nil
}

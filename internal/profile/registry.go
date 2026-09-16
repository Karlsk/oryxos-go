package profile

// Registry is an immutable startup snapshot indexed by Profile.name.
type Registry struct {
	profiles map[string]*Profile
	names    []string
}

// ProfileRegistry is the explicit architecture name for a Profile registry.
// Registry remains the concise package-local spelling used by existing callers.
type ProfileRegistry = Registry

func newRegistry(profiles map[string]*Profile, names []string) *Registry {
	cloned := make(map[string]*Profile, len(profiles))
	for name, profileValue := range profiles {
		cloned[name] = cloneProfile(profileValue)
	}
	return &Registry{profiles: cloned, names: append([]string(nil), names...)}
}

// Get returns the Profile registered under name.
func (registry *Registry) Get(name string) (*Profile, bool) {
	if registry == nil {
		return nil, false
	}
	profileValue, ok := registry.profiles[name]
	return cloneProfile(profileValue), ok
}

// Len returns the number of valid Profiles in the snapshot.
func (registry *Registry) Len() int {
	if registry == nil {
		return 0
	}
	return len(registry.profiles)
}

// List returns Profiles in deterministic lexical-file registration order.
func (registry *Registry) List() []*Profile {
	if registry == nil {
		return nil
	}
	result := make([]*Profile, 0, len(registry.names))
	for _, name := range registry.names {
		result = append(result, cloneProfile(registry.profiles[name]))
	}
	return result
}

func cloneProfile(source *Profile) *Profile {
	if source == nil {
		return nil
	}
	cloned := *source
	cloned.Tools = append([]string(nil), source.Tools...)
	cloned.Skills = append([]string(nil), source.Skills...)
	cloned.MCPServers = append([]string(nil), source.MCPServers...)
	cloned.NotifyChannels = append([]NotifyChannelConfig(nil), source.NotifyChannels...)
	cloned.Schedules = append([]ScheduleConfig(nil), source.Schedules...)
	cloned.Channels = cloneChannels(source.Channels)
	cloned.Bootstrap = append([]string(nil), source.Bootstrap...)
	return &cloned
}

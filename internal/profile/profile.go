// Package profile loads immutable Agent runtime selections from Profile YAML.
package profile

// Profile is the complete core-stage structural Profile container.
type Profile struct {
	Name           string                `yaml:"name"`
	Description    string                `yaml:"description"`
	Identity       IdentityConfig        `yaml:"identity"`
	Provider       ProviderConfig        `yaml:"provider"`
	Tools          []string              `yaml:"tools"`
	Skills         []string              `yaml:"skills"`
	MCPServers     []string              `yaml:"mcp_servers"`
	NotifyChannels []NotifyChannelConfig `yaml:"notify_channels"`
	Schedules      []ScheduleConfig      `yaml:"schedules"`
	Channels       []ChannelConfig       `yaml:"channels"`
	Bootstrap      []string              `yaml:"bootstrap"`
	Settings       SettingsConfig        `yaml:"settings"`
}

// IdentityConfig contains the display identity and baseline prompt.
type IdentityConfig struct {
	AgentName string `yaml:"agent_name"`
	Prompt    string `yaml:"prompt"`
}

// ProviderConfig selects how this Profile uses a process-declared Provider.
type ProviderConfig struct {
	Name        string  `yaml:"name"`
	Model       string  `yaml:"model"`
	Temperature float32 `yaml:"temperature"`
}

// NotifyChannelConfig is structurally loaded for the notification lesson.
type NotifyChannelConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	URL  string `yaml:"url"`
}

// ScheduleConfig is structurally loaded for the scheduler lesson.
type ScheduleConfig struct {
	ID       string `yaml:"id"`
	Cron     string `yaml:"cron"`
	Timezone string `yaml:"timezone"`
	Message  string `yaml:"message"`
	Enabled  bool   `yaml:"enabled"`
}

// ChannelConfig is structurally loaded for Channel assembly.
type ChannelConfig struct {
	Name   string         `yaml:"name"`
	Config map[string]any `yaml:"config"`
}

// SettingsConfig contains bounded runtime settings validated by later lessons.
type SettingsConfig struct {
	MaxIterations   int `yaml:"max_iterations"`
	MaxHistoryTurns int `yaml:"max_history_turns"`
}

type rawProfile struct {
	Name           string                `yaml:"name"`
	Description    string                `yaml:"description"`
	Identity       IdentityConfig        `yaml:"identity"`
	Provider       rawProviderConfig     `yaml:"provider"`
	Tools          []string              `yaml:"tools"`
	Skills         []string              `yaml:"skills"`
	MCPServers     []string              `yaml:"mcp_servers"`
	NotifyChannels []NotifyChannelConfig `yaml:"notify_channels"`
	Schedules      []ScheduleConfig      `yaml:"schedules"`
	Channels       []ChannelConfig       `yaml:"channels"`
	Bootstrap      []string              `yaml:"bootstrap"`
	Settings       rawSettingsConfig     `yaml:"settings"`
}

type rawProviderConfig struct {
	Name        string   `yaml:"name"`
	Model       string   `yaml:"model"`
	Temperature *float32 `yaml:"temperature"`
}

type rawSettingsConfig struct {
	MaxIterations   *int `yaml:"max_iterations"`
	MaxHistoryTurns *int `yaml:"max_history_turns"`
}

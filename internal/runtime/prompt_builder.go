package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Karlsk/oryxos-go/internal/bootstrap"
	"github.com/Karlsk/oryxos-go/internal/llm"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/Karlsk/oryxos-go/internal/session"
	"github.com/Karlsk/oryxos-go/internal/skill"
)

const (
	defaultHistoryTurns = 20
	maxMemoryCharacters = 4000
)

type memoryReader interface {
	Read(ctx context.Context, selected *profile.Profile) (string, error)
}

// PromptBuilder assembles immutable startup context with request-time Memory,
// conversation history, the current request, and an injected clock.
type PromptBuilder struct {
	runtimeRules       string
	bootstrapByProfile map[string]bootstrap.Snapshot
	skillsByProfile    map[string]skill.Snapshot
	maxInputRunes      map[string]int
	memory             memoryReader
	now                func() time.Time
}

// NewPromptBuilder constructs a deterministic context builder.
func NewPromptBuilder(runtimeRules string, bootstrapByProfile map[string]bootstrap.Snapshot, skillsByProfile map[string]skill.Snapshot, memory memoryReader, maxInputRunes map[string]int, now func() time.Time) (*PromptBuilder, error) {
	if strings.TrimSpace(runtimeRules) == "" {
		return nil, fmt.Errorf("create prompt builder: runtime rules are empty")
	}
	if bootstrapByProfile == nil {
		return nil, fmt.Errorf("create prompt builder: bootstrap snapshots are nil")
	}
	if skillsByProfile == nil {
		return nil, fmt.Errorf("create prompt builder: skill snapshots are nil")
	}
	if memory == nil {
		return nil, fmt.Errorf("create prompt builder: memory reader is nil")
	}
	if maxInputRunes == nil {
		return nil, fmt.Errorf("create prompt builder: input budgets are nil")
	}
	for name := range bootstrapByProfile {
		if maxInputRunes[name] <= 0 {
			return nil, fmt.Errorf("create prompt builder: input budget for profile %q must be positive", name)
		}
	}
	if now == nil {
		return nil, fmt.Errorf("create prompt builder: clock is nil")
	}
	return &PromptBuilder{
		runtimeRules:       runtimeRules,
		bootstrapByProfile: cloneBootstrapSnapshots(bootstrapByProfile),
		skillsByProfile:    cloneSkillSnapshots(skillsByProfile),
		maxInputRunes:      cloneInputBudgets(maxInputRunes),
		memory:             memory,
		now:                now,
	}, nil
}

// Build preserves structured history messages while surrounding every source
// with explicit provenance labels.
func (builder *PromptBuilder) Build(ctx context.Context, current *session.Session, selected *profile.Profile, currentUserMessage string) ([]llm.Message, error) {
	if builder == nil || builder.memory == nil || builder.now == nil {
		return nil, fmt.Errorf("build prompt: builder is not initialized")
	}
	if ctx == nil {
		return nil, fmt.Errorf("build prompt: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("build prompt: session is nil")
	}
	if selected == nil || strings.TrimSpace(selected.Name) == "" {
		return nil, fmt.Errorf("build prompt: profile is required")
	}
	boot, ok := builder.bootstrapByProfile[selected.Name]
	if !ok {
		return nil, fmt.Errorf("build prompt: bootstrap snapshot for profile %q not found", selected.Name)
	}
	skills, ok := builder.skillsByProfile[selected.Name]
	if !ok {
		return nil, fmt.Errorf("build prompt: skill snapshot for profile %q not found", selected.Name)
	}
	budget := builder.maxInputRunes[selected.Name]
	if budget <= 0 {
		return nil, fmt.Errorf("build prompt: input budget for profile %q not found", selected.Name)
	}
	memory, err := builder.memory.Read(ctx, selected)
	if err != nil {
		return nil, fmt.Errorf("build prompt: read memory: %w", err)
	}
	memory = truncateRunes(memory, maxMemoryCharacters)

	projectRules, persona, preferences := classifyBootstrap(boot.Sections())
	identity := strings.TrimSpace(strings.Join(nonEmpty(selected.Identity.AgentName, selected.Identity.Prompt, persona), "\n"))
	systemContent := strings.Join([]string{
		"[RUNTIME_RULES]\n" + builder.runtimeRules,
		"[PROJECT_RULES]\n" + projectRules,
		"[AGENT_IDENTITY]\n" + identity,
		"[SKILLS]\n" + renderSkillSections(skills.Sections()),
		"[USER_PREFERENCES]\n" + preferences,
		"[LONG_TERM_MEMORY]\n" + memory,
	}, "\n")

	prefix := []llm.Message{
		{Role: llm.RoleSystem, Content: systemContent},
		{Role: llm.RoleSystem, Content: "[HISTORY]"},
	}
	history, currentTurn := splitCurrentTurn(current.Messages(), currentUserMessage)
	history = limitCompleteTurns(history, historyLimit(selected))
	suffix := []llm.Message{{Role: llm.RoleSystem, Content: "[USER_MESSAGE]"}}
	suffix = append(suffix, currentTurn...)
	suffix = append(suffix,
		llm.Message{Role: llm.RoleSystem, Content: "[CURRENT_DATETIME]\n" + builder.now().Format(time.RFC3339)},
	)

	for {
		messages := make([]llm.Message, 0, len(prefix)+len(history)+len(suffix))
		messages = append(messages, prefix...)
		messages = append(messages, history...)
		messages = append(messages, suffix...)
		if messagesRuneCount(messages) <= budget {
			return messages, nil
		}
		if len(history) == 0 {
			return nil, fmt.Errorf("build prompt: protected context for profile %q exceeds input budget %d", selected.Name, budget)
		}
		history = dropOldestCompleteTurn(history)
	}
}

func cloneInputBudgets(source map[string]int) map[string]int {
	cloned := make(map[string]int, len(source))
	for name, budget := range source {
		cloned[name] = budget
	}
	return cloned
}

func cloneBootstrapSnapshots(source map[string]bootstrap.Snapshot) map[string]bootstrap.Snapshot {
	cloned := make(map[string]bootstrap.Snapshot, len(source))
	for name, snapshot := range source {
		cloned[name] = bootstrap.NewSnapshot(snapshot.Sections())
	}
	return cloned
}

func cloneSkillSnapshots(source map[string]skill.Snapshot) map[string]skill.Snapshot {
	cloned := make(map[string]skill.Snapshot, len(source))
	for name, snapshot := range source {
		cloned[name] = skill.NewSnapshot(snapshot.Sections())
	}
	return cloned
}

func classifyBootstrap(sections []bootstrap.Section) (projectRules, persona, preferences string) {
	var project []bootstrap.Section
	for _, section := range sections {
		switch strings.ToUpper(filepath.Base(section.Reference)) {
		case "SOUL.MD":
			persona = section.Content
		case "USER.MD":
			preferences = section.Content
		default:
			project = append(project, section)
		}
	}
	return renderBootstrapSections(project), persona, preferences
}

func renderBootstrapSections(sections []bootstrap.Section) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		parts = append(parts, "--- BOOTSTRAP: "+section.Reference+" ---\n"+section.Content)
	}
	return strings.Join(parts, "\n")
}

func renderSkillSections(sections []skill.Section) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		parts = append(parts, "--- SKILL: "+section.Reference+" ---\n"+section.Content)
	}
	return strings.Join(parts, "\n")
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func historyLimit(selected *profile.Profile) int {
	if selected.Settings.MaxHistoryTurns > 0 {
		return selected.Settings.MaxHistoryTurns
	}
	return defaultHistoryTurns
}

func splitCurrentTurn(messages []llm.Message, currentUserMessage string) ([]llm.Message, []llm.Message) {
	currentStart := -1
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == llm.RoleUser && messages[index].Content == currentUserMessage {
			currentStart = index
			break
		}
	}
	if currentStart < 0 {
		return messages, []llm.Message{{Role: llm.RoleUser, Content: currentUserMessage}}
	}
	return messages[:currentStart], messages[currentStart:]
}

func limitCompleteTurns(messages []llm.Message, limit int) []llm.Message {
	starts := make([]int, 0)
	for index, message := range messages {
		if message.Role == llm.RoleUser {
			starts = append(starts, index)
		}
	}
	if len(starts) == 0 || limit <= 0 {
		return nil
	}
	turn := 0
	if len(starts) > limit {
		turn = len(starts) - limit
	}
	return messages[starts[turn]:]
}

func dropOldestCompleteTurn(messages []llm.Message) []llm.Message {
	foundFirst := false
	for index, message := range messages {
		if message.Role != llm.RoleUser {
			continue
		}
		if foundFirst {
			return messages[index:]
		}
		foundFirst = true
	}
	return nil
}

func messagesRuneCount(messages []llm.Message) int {
	total := 0
	for _, message := range messages {
		total += utf8.RuneCountInString(string(message.Role))
		total += utf8.RuneCountInString(message.Content)
		total += utf8.RuneCountInString(message.Name)
		total += utf8.RuneCountInString(message.ToolCallID)
		total += utf8.RuneCountInString(message.ToolName)
		total += utf8.RuneCountInString(message.ReasoningContent)
		for _, call := range message.ToolCalls {
			total += utf8.RuneCountInString(call.ID)
			total += utf8.RuneCountInString(call.Type)
			total += utf8.RuneCountInString(call.Function.Name)
			total += utf8.RuneCountInString(call.Function.Arguments)
			total += jsonRuneCount(call.Extra)
		}
		total += jsonRuneCount(message.Extra)
	}
	return total
}

func jsonRuneCount(value map[string]any) int {
	if len(value) == 0 {
		return 0
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return utf8.RuneCount(encoded)
}

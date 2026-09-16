package provider

// Supported explicit Provider names.
const (
	DeepSeek = "deepseek"
	MiniMax  = "minimax"
)

const miniMaxOpenAIBaseURL = "https://api.minimax.cn/v1"

// ProviderConfig is the ephemeral endpoint-free input to one ModelFactory.
type ProviderConfig struct {
	Name        string
	Model       string
	APIKey      string
	Temperature float32
}

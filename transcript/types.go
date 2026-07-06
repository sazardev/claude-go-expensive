// Package transcript parses Claude Code session transcripts — the JSONL
// files Claude Code writes under ~/.claude/projects/<project>/<session>.jsonl.
package transcript

import (
	"encoding/json"
	"strings"
	"time"
)

// Entry is one line of a session transcript.
type Entry struct {
	Type        string    `json:"type"` // "user", "assistant", "summary", "system"
	UUID        string    `json:"uuid"`
	ParentUUID  string    `json:"parentUuid"`
	SessionID   string    `json:"sessionId"`
	Cwd         string    `json:"cwd"`
	GitBranch   string    `json:"gitBranch"`
	IsSidechain bool      `json:"isSidechain"`
	Timestamp   time.Time `json:"timestamp"`
	Message     *Message  `json:"message"`
}

// Message is the assistant/user payload embedded in an Entry.
type Message struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *Usage          `json:"usage"`
}

// Usage is the token accounting the API attaches to an assistant message.
type Usage struct {
	InputTokens              int64          `json:"input_tokens"`
	OutputTokens             int64          `json:"output_tokens"`
	CacheCreationInputTokens int64          `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64          `json:"cache_read_input_tokens"`
	CacheCreation            *CacheCreation `json:"cache_creation"`
	ServerToolUse            *ServerToolUse `json:"server_tool_use"`
	ServiceTier              string         `json:"service_tier"`
	Speed                    string         `json:"speed"`
}

// CacheCreation splits cache-write tokens by TTL — 5-minute and 1-hour
// writes are billed at different rates, so this is not derivable from
// Usage.CacheCreationInputTokens alone.
type CacheCreation struct {
	Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"`
}

// ServerToolUse counts server-side tool invocations billed outside normal
// token pricing (web_fetch is free; web_search is $10/1,000 requests).
type ServerToolUse struct {
	WebSearchRequests int64 `json:"web_search_requests"`
	WebFetchRequests  int64 `json:"web_fetch_requests"`
}

// Block is one element of Message.Content when Content is a content-block
// array (as opposed to a plain string).
type Block struct {
	Type      string          `json:"type"` // "text", "tool_use", "tool_result"
	Text      string          `json:"text"`
	Name      string          `json:"name"`        // tool name, for "tool_use"
	ID        string          `json:"id"`          // this block's own id, for "tool_use" (e.g. "toolu_...")
	Input     json.RawMessage `json:"input"`       // tool input, for "tool_use"
	ToolUseID string          `json:"tool_use_id"` // the tool_use id this result answers, for "tool_result"
}

// Blocks parses m.Content into content blocks. A plain string content (the
// common shape for simple user turns) is returned as a single text block.
func (m *Message) Blocks() []Block {
	if m == nil || len(m.Content) == 0 {
		return nil
	}
	var blocks []Block
	if err := json.Unmarshal(m.Content, &blocks); err == nil {
		return blocks
	}
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil && text != "" {
		return []Block{{Type: "text", Text: text}}
	}
	return nil
}

// Text concatenates the text blocks of the message.
func (m *Message) Text() string {
	var sb strings.Builder
	for _, b := range m.Blocks() {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// IsToolResultOnly reports whether the message consists entirely of
// tool_result blocks — the synthetic "user" turn Claude Code inserts to
// relay tool output back to the model, not an actual user prompt.
func (m *Message) IsToolResultOnly() bool {
	blocks := m.Blocks()
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			return false
		}
	}
	return true
}

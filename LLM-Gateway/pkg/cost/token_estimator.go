// Package cost provides token estimation for different LLM providers
package cost

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/llm-gateway/gateway/pkg/types"
)

// TokenEstimator estimates token counts for different providers and models
type TokenEstimator struct {
	// Provider-specific estimation rules
	estimationRules map[string]*EstimationRule
}

// EstimationRule defines token estimation parameters for different providers
type EstimationRule struct {
	Provider        string  `json:"provider"`
	CharsPerToken   float64 `json:"chars_per_token"`  // Average characters per token
	WordsPerToken   float64 `json:"words_per_token"`  // Average words per token
	SystemTokens    int     `json:"system_tokens"`    // Fixed tokens for system messages
	MessageOverhead int     `json:"message_overhead"` // Overhead tokens per message
	ModelMultiplier float64 `json:"model_multiplier"` // Model-specific multiplier
}

// NewTokenEstimator creates a new token estimator with default rules
func NewTokenEstimator() *TokenEstimator {
	te := &TokenEstimator{
		estimationRules: make(map[string]*EstimationRule),
	}

	// Initialize default estimation rules
	te.initializeEstimationRules()

	return te
}

// EstimateTokens estimates token count for a chat completion request
func (te *TokenEstimator) EstimateTokens(req *types.ChatCompletionRequest, provider string) (*types.TokenEstimate, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	rule := te.getEstimationRule(provider)

	// Calculate input tokens
	inputTokens := 0

	// Add system tokens if there are system messages
	hasSystemMessage := false
	for _, message := range req.Messages {
		if message.Role == "system" {
			hasSystemMessage = true
			break
		}
	}
	if hasSystemMessage {
		inputTokens += rule.SystemTokens
	}

	// Calculate tokens for each message
	for _, message := range req.Messages {
		messageTokens := te.estimateMessageTokens(message.Content, rule)
		inputTokens += messageTokens + rule.MessageOverhead
	}

	// Apply model-specific multiplier
	inputTokens = int(float64(inputTokens) * rule.ModelMultiplier)

	// Estimate output tokens based on max_tokens or default
	var outputTokens int
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		outputTokens = *req.MaxTokens
	} else {
		// Default output token estimation
		outputTokens = te.estimateDefaultOutputTokens(inputTokens, provider)
	}

	// Ensure minimum token counts
	if inputTokens < 1 {
		inputTokens = 1
	}
	if outputTokens < 1 {
		outputTokens = 1
	}

	estimate := &types.TokenEstimate{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
	}

	return estimate, nil
}

// EstimateOutputTokens estimates tokens for response content
func (te *TokenEstimator) EstimateOutputTokens(content, provider string) int {
	rule := te.getEstimationRule(provider)
	return te.estimateMessageTokens(content, rule)
}

// estimateMessageTokens estimates tokens for a single message
func (te *TokenEstimator) estimateMessageTokens(content string, rule *EstimationRule) int {
	if content == "" {
		return 0
	}

	// Method 1: Character-based estimation
	charCount := utf8.RuneCountInString(content)
	tokensByChars := int(float64(charCount) / rule.CharsPerToken)

	// Method 2: Word-based estimation
	wordCount := te.countWords(content)
	tokensByWords := int(float64(wordCount) / rule.WordsPerToken)

	// Use the higher estimate for safety
	tokens := tokensByChars
	if tokensByWords > tokens {
		tokens = tokensByWords
	}

	// Special handling for different content types
	if te.containsCode(content) {
		// Code typically has more tokens per character
		tokens = int(float64(tokens) * 1.3)
	}

	if te.containsSpecialChars(content) {
		// Special characters and symbols may increase token count
		tokens = int(float64(tokens) * 1.1)
	}

	return tokens
}

// countWords counts the number of words in a text
func (te *TokenEstimator) countWords(text string) int {
	// Split by common word separators
	words := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' ||
			r == '.' || r == ',' || r == '!' || r == '?' ||
			r == ';' || r == ':'
	})

	// Filter out empty strings
	wordCount := 0
	for _, word := range words {
		if strings.TrimSpace(word) != "" {
			wordCount++
		}
	}

	return wordCount
}

// containsCode checks if the content contains code patterns
func (te *TokenEstimator) containsCode(content string) bool {
	codeIndicators := []string{
		"```", "function", "def ", "class ", "import ", "from ",
		"const ", "let ", "var ", "if (", "for (", "while (",
		"return ", "{", "}", "[", "]", "//", "/*", "*/",
	}

	contentLower := strings.ToLower(content)
	for _, indicator := range codeIndicators {
		if strings.Contains(contentLower, strings.ToLower(indicator)) {
			return true
		}
	}

	return false
}

// containsSpecialChars checks if content has many special characters
func (te *TokenEstimator) containsSpecialChars(content string) bool {
	specialCount := 0
	totalCount := utf8.RuneCountInString(content)

	if totalCount == 0 {
		return false
	}

	for _, r := range content {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ' ' || r == '\n' || r == '\t') {
			specialCount++
		}
	}

	// If more than 20% special characters, consider it special content
	return float64(specialCount)/float64(totalCount) > 0.2
}

// estimateDefaultOutputTokens estimates default output tokens based on input
func (te *TokenEstimator) estimateDefaultOutputTokens(inputTokens int, provider string) int {
	// Default output estimation based on provider and input length
	switch provider {
	case "openai":
		// OpenAI models typically generate reasonable length responses
		if inputTokens < 100 {
			return 150 // Short response
		} else if inputTokens < 1000 {
			return int(float64(inputTokens) * 0.5) // About half the input length
		} else {
			return 500 // Cap for long inputs
		}

	case "anthropic":
		// Claude tends to generate longer, more detailed responses
		if inputTokens < 100 {
			return 200
		} else if inputTokens < 1000 {
			return int(float64(inputTokens) * 0.7)
		} else {
			return 700
		}

	case "baidu":
		// Conservative estimation for Baidu models
		if inputTokens < 100 {
			return 100
		} else if inputTokens < 1000 {
			return int(float64(inputTokens) * 0.4)
		} else {
			return 400
		}

	default:
		// Default conservative estimation
		return int(float64(inputTokens) * 0.5)
	}
}

// getEstimationRule returns the estimation rule for a provider
func (te *TokenEstimator) getEstimationRule(provider string) *EstimationRule {
	if rule, exists := te.estimationRules[provider]; exists {
		return rule
	}

	// Return default rule if provider not found
	return te.estimationRules["default"]
}

// initializeEstimationRules initializes default estimation rules for different providers
func (te *TokenEstimator) initializeEstimationRules() {
	// OpenAI estimation rules (based on tiktoken analysis)
	te.estimationRules["openai"] = &EstimationRule{
		Provider:        "openai",
		CharsPerToken:   4.0,  // GPT models average ~4 characters per token
		WordsPerToken:   0.75, // ~0.75 words per token
		SystemTokens:    3,    // System message overhead
		MessageOverhead: 4,    // Per message overhead (role, formatting)
		ModelMultiplier: 1.0,  // Base multiplier
	}

	// Anthropic estimation rules (Claude tokenization)
	te.estimationRules["anthropic"] = &EstimationRule{
		Provider:        "anthropic",
		CharsPerToken:   4.2, // Claude slightly higher chars per token
		WordsPerToken:   0.8, // ~0.8 words per token
		SystemTokens:    5,   // System message overhead
		MessageOverhead: 6,   // Higher overhead for Claude format
		ModelMultiplier: 1.1, // Slightly higher token count
	}

	// Baidu estimation rules (Chinese optimized)
	te.estimationRules["baidu"] = &EstimationRule{
		Provider:        "baidu",
		CharsPerToken:   2.5, // Chinese characters are more token-dense
		WordsPerToken:   0.6, // Chinese word segmentation
		SystemTokens:    2,   // System message overhead
		MessageOverhead: 3,   // Per message overhead
		ModelMultiplier: 1.2, // Account for Chinese tokenization
	}

	// Default estimation rule
	te.estimationRules["default"] = &EstimationRule{
		Provider:        "default",
		CharsPerToken:   4.0,
		WordsPerToken:   0.75,
		SystemTokens:    3,
		MessageOverhead: 4,
		ModelMultiplier: 1.0,
	}
}

// UpdateEstimationRule updates or adds an estimation rule for a provider
func (te *TokenEstimator) UpdateEstimationRule(provider string, rule *EstimationRule) {
	te.estimationRules[provider] = rule
}

// GetEstimationRule returns the estimation rule for a provider (for debugging/testing)
func (te *TokenEstimator) GetEstimationRule(provider string) *EstimationRule {
	return te.getEstimationRule(provider)
}

// ValidateTokenLimits checks if a request would exceed token limits
func (te *TokenEstimator) ValidateTokenLimits(estimate *types.TokenEstimate, maxTokens int) error {
	if maxTokens > 0 && estimate.TotalTokens > maxTokens {
		return fmt.Errorf("estimated total tokens %d exceeds limit %d",
			estimate.TotalTokens, maxTokens)
	}

	return nil
}

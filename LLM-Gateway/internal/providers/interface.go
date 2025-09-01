// Package providers defines interfaces and implementations for LLM providers
package providers

import (
	"github.com/llm-gateway/gateway/pkg/types"
)

// ProviderError represents errors from provider operations
type ProviderError struct {
	Provider   string `json:"provider"`
	Operation  string `json:"operation"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
}

func (e *ProviderError) Error() string {
	return e.Message
}

// ProviderRegistry manages all registered providers
type ProviderRegistry interface {
	// RegisterProvider registers a new provider
	RegisterProvider(provider types.Provider) error

	// GetProvider returns a provider by name
	GetProvider(name string) (types.Provider, error)

	// GetProviders returns all registered providers
	GetProviders() []types.Provider

	// GetHealthyProviders returns only healthy providers
	GetHealthyProviders() []types.Provider

	// GetProvidersByType returns providers of a specific type
	GetProvidersByType(providerType string) []types.Provider
}

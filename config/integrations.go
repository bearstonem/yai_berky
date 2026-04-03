package config

import (
	"encoding/json"

	"github.com/spf13/viper"
)

const integrations_key = "INTEGRATIONS"

// IntegrationType identifies a known integration.
type IntegrationType string

const (
	IntegrationComfyUI IntegrationType = "comfyui"
	IntegrationWebhook IntegrationType = "webhook"
)

// IntegrationConfig stores the configuration for a single external tool integration.
type IntegrationConfig struct {
	Type     IntegrationType `json:"type"`
	Name     string          `json:"name"`
	Endpoint string          `json:"endpoint"`
	APIKey   string          `json:"api_key,omitempty"`
	// ComfyUI-specific: workflow JSON (the API-format prompt)
	Workflow json.RawMessage `json:"workflow,omitempty"`
	// Webhook-specific: HTTP method
	Method string `json:"method,omitempty"`
	// Generic metadata
	Enabled bool `json:"enabled"`
}

// IntegrationTypeInfo describes an available integration type for interactive setup.
type IntegrationTypeInfo struct {
	Type        IntegrationType
	DisplayName string
	Description string
	NeedsAPIKey bool
	NeedsWorkflow bool
}

// AvailableIntegrations returns the list of supported integration types.
func AvailableIntegrations() []IntegrationTypeInfo {
	return []IntegrationTypeInfo{
		{
			Type:          IntegrationComfyUI,
			DisplayName:   "ComfyUI",
			Description:   "Generate images via a ComfyUI server (local or remote)",
			NeedsAPIKey:   false,
			NeedsWorkflow: true,
		},
		{
			Type:          IntegrationWebhook,
			DisplayName:   "Webhook",
			Description:   "Call an arbitrary HTTP endpoint as a tool",
			NeedsAPIKey:   true,
			NeedsWorkflow: false,
		},
	}
}

// LoadIntegrationsFromViper reads the INTEGRATIONS config key.
func LoadIntegrationsFromViper() []IntegrationConfig {
	raw := viper.GetString(integrations_key)
	if raw == "" {
		return nil
	}
	var integrations []IntegrationConfig
	if err := json.Unmarshal([]byte(raw), &integrations); err != nil {
		return nil
	}
	return integrations
}

// SaveIntegrationsToViper writes integrations to viper (call viper.WriteConfig after).
func SaveIntegrationsToViper(integrations []IntegrationConfig) {
	data, err := json.Marshal(integrations)
	if err != nil {
		return
	}
	viper.Set(integrations_key, string(data))
}

// AddIntegration appends an integration and persists to viper.
func AddIntegration(ic IntegrationConfig) {
	existing := LoadIntegrationsFromViper()
	existing = append(existing, ic)
	SaveIntegrationsToViper(existing)
}

// RemoveIntegration removes an integration by name and persists.
func RemoveIntegration(name string) bool {
	existing := LoadIntegrationsFromViper()
	found := false
	filtered := existing[:0]
	for _, ic := range existing {
		if ic.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, ic)
	}
	if found {
		SaveIntegrationsToViper(filtered)
	}
	return found
}

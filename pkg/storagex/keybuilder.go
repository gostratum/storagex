package storagex

import (
	"fmt"
	"strings"
)

// KeyBuilder defines the interface for building object keys with optional prefixes
type KeyBuilder interface {
	// BuildKey constructs the final object key, potentially adding prefixes
	BuildKey(originalKey string, context map[string]string) string

	// StripKey removes any prefixes to get the original key
	StripKey(fullKey string) string
}

// PrefixKeyBuilder implements KeyBuilder with configurable prefix templates
type PrefixKeyBuilder struct {
	// BasePrefix is a template string that can contain placeholders like %s
	// Example: "org/%s/workspace/%s"
	BasePrefix string

	// Separator is used to join prefix parts (default: "/")
	Separator string
}

// NewPrefixKeyBuilder creates a new PrefixKeyBuilder with the given base prefix
func NewPrefixKeyBuilder(basePrefix string) *PrefixKeyBuilder {
	return &PrefixKeyBuilder{
		BasePrefix: basePrefix,
		Separator:  "/",
	}
}

// BuildKey constructs a key by applying the prefix template with context values
func (kb *PrefixKeyBuilder) BuildKey(originalKey string, context map[string]string) string {
	if kb.BasePrefix == "" {
		return originalKey
	}

	prefix := kb.buildPrefix(context)
	if prefix == "" {
		return originalKey
	}

	// Ensure clean key joining
	prefix = strings.TrimSuffix(prefix, kb.Separator)
	originalKey = strings.TrimPrefix(originalKey, kb.Separator)

	return fmt.Sprintf("%s%s%s", prefix, kb.Separator, originalKey)
}

// StripKey removes the prefix to return the original key
func (kb *PrefixKeyBuilder) StripKey(fullKey string) string {
	if kb.BasePrefix == "" {
		return fullKey
	}

	// For simplicity, we'll look for the last separator and take everything after it
	// In a production implementation, you might want more sophisticated logic
	parts := strings.Split(fullKey, kb.Separator)
	if len(parts) <= 1 {
		return fullKey
	}

	// Return the last part as the original key
	return parts[len(parts)-1]
}

// buildPrefix constructs the prefix from the template and context
func (kb *PrefixKeyBuilder) buildPrefix(context map[string]string) string {
	if kb.BasePrefix == "" {
		return ""
	}

	// If no context provided and prefix has no placeholders, use prefix as-is
	if context == nil {
		// Check if prefix contains placeholders
		if !strings.Contains(kb.BasePrefix, "{") && !strings.Contains(kb.BasePrefix, "%s") {
			return kb.BasePrefix
		}
		return ""
	}

	// Simple template replacement - in production you might want a more robust solution
	prefix := kb.BasePrefix

	// Replace common placeholders
	if orgID, exists := context["org_id"]; exists {
		prefix = strings.ReplaceAll(prefix, "{org_id}", orgID)
		prefix = strings.ReplaceAll(prefix, "%s", orgID) // Support printf-style too
	}

	if workspaceID, exists := context["workspace_id"]; exists {
		// If we have both org and workspace, we need ordered replacement
		if strings.Contains(prefix, "{workspace_id}") {
			prefix = strings.ReplaceAll(prefix, "{workspace_id}", workspaceID)
		} else if strings.Count(prefix, "%s") >= 2 {
			// For printf-style with multiple %s, we'll need proper formatting
			if orgID, hasOrg := context["org_id"]; hasOrg {
				prefix = fmt.Sprintf(kb.BasePrefix, orgID, workspaceID)
			}
		}
	}

	if userID, exists := context["user_id"]; exists {
		prefix = strings.ReplaceAll(prefix, "{user_id}", userID)
	}

	if env, exists := context["environment"]; exists {
		prefix = strings.ReplaceAll(prefix, "{environment}", env)
	}

	return prefix
}

// TenantKeyBuilder is a specialized KeyBuilder for multi-tenant scenarios
type TenantKeyBuilder struct {
	*PrefixKeyBuilder
	TenantID string // Fixed tenant ID for this instance
}

// NewTenantKeyBuilder creates a KeyBuilder for a specific tenant
func NewTenantKeyBuilder(tenantID string, baseTemplate string) *TenantKeyBuilder {
	return &TenantKeyBuilder{
		PrefixKeyBuilder: NewPrefixKeyBuilder(baseTemplate),
		TenantID:         tenantID,
	}
}

// BuildKey builds a key with the tenant context automatically added
func (tkb *TenantKeyBuilder) BuildKey(originalKey string, context map[string]string) string {
	if context == nil {
		context = make(map[string]string)
	}

	// Always set the tenant ID
	if tkb.TenantID != "" {
		context["tenant_id"] = tkb.TenantID
		context["org_id"] = tkb.TenantID // Alias for org_id
	}

	return tkb.PrefixKeyBuilder.BuildKey(originalKey, context)
}

// DatePartitionedKeyBuilder adds date-based partitioning to keys
type DatePartitionedKeyBuilder struct {
	*PrefixKeyBuilder
	DateFormat string        // Format for date partitioning (e.g., "2006/01/02")
	GetTime    func() string // Function to get current time string
}

// NewDatePartitionedKeyBuilder creates a KeyBuilder with date partitioning
func NewDatePartitionedKeyBuilder(basePrefix, dateFormat string, getTime func() string) *DatePartitionedKeyBuilder {
	if dateFormat == "" {
		dateFormat = "2006/01/02" // Default: year/month/day
	}

	return &DatePartitionedKeyBuilder{
		PrefixKeyBuilder: NewPrefixKeyBuilder(basePrefix),
		DateFormat:       dateFormat,
		GetTime:          getTime,
	}
}

// BuildKey builds a key with date partitioning
func (dpkb *DatePartitionedKeyBuilder) BuildKey(originalKey string, context map[string]string) string {
	if context == nil {
		context = make(map[string]string)
	}

	// Add date partition
	if dpkb.GetTime != nil {
		context["date"] = dpkb.GetTime()
	}

	return dpkb.PrefixKeyBuilder.BuildKey(originalKey, context)
}

// NoOpKeyBuilder implements KeyBuilder but performs no transformations
type NoOpKeyBuilder struct{}

// BuildKey returns the original key unchanged
func (nkb *NoOpKeyBuilder) BuildKey(originalKey string, context map[string]string) string {
	return originalKey
}

// StripKey returns the key unchanged
func (nkb *NoOpKeyBuilder) StripKey(fullKey string) string {
	return fullKey
}

// KeyBuilderChain allows chaining multiple KeyBuilders
type KeyBuilderChain struct {
	builders []KeyBuilder
}

// NewKeyBuilderChain creates a chain of KeyBuilders
func NewKeyBuilderChain(builders ...KeyBuilder) *KeyBuilderChain {
	return &KeyBuilderChain{
		builders: builders,
	}
}

// BuildKey applies all builders in sequence
func (kbc *KeyBuilderChain) BuildKey(originalKey string, context map[string]string) string {
	key := originalKey
	for _, builder := range kbc.builders {
		key = builder.BuildKey(key, context)
	}
	return key
}

// StripKey applies all builders in reverse sequence
func (kbc *KeyBuilderChain) StripKey(fullKey string) string {
	key := fullKey
	// Apply stripping in reverse order
	for i := len(kbc.builders) - 1; i >= 0; i-- {
		key = kbc.builders[i].StripKey(key)
	}
	return key
}

package handlers

import (
	"fmt"
	"testing"
	"time"
)

// Test functions for applyURLTemplate
func TestApplyURLTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		envVars     map[string]string
		expected    string
		expectError bool
	}{
		{
			name:     "basic substitution",
			template: "https://${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}",
			envVars: map[string]string{
				"DATABRICKS_WORKSPACE_URL":  "workspace.cloud.databricks.com",
				"DATABRICKS_GENIE_SPACE_ID": "12345",
			},
			expected:    "https://workspace.cloud.databricks.com/api/2.0/mcp/genie/12345",
			expectError: false,
		},
		{
			name:     "single variable",
			template: "https://${API_HOST}/v1/endpoint",
			envVars: map[string]string{
				"API_HOST": "api.example.com",
			},
			expected:    "https://api.example.com/v1/endpoint",
			expectError: false,
		},
		{
			name:        "no variables",
			template:    "https://example.com/api",
			envVars:     map[string]string{},
			expected:    "https://example.com/api",
			expectError: false,
		},
		{
			name:        "empty template",
			template:    "",
			envVars:     map[string]string{},
			expected:    "",
			expectError: false,
		},
		{
			name:     "variable with special characters",
			template: "https://${API_HOST}/path/${USER_ID}/data",
			envVars: map[string]string{
				"API_HOST": "api.example.com",
				"USER_ID":  "user-123_456",
			},
			expected:    "https://api.example.com/path/user-123_456/data",
			expectError: false,
		},
		{
			name:     "multiple same variable",
			template: "https://${API_HOST}/api/${API_HOST}/status",
			envVars: map[string]string{
				"API_HOST": "api.example.com",
			},
			expected:    "https://api.example.com/api/api.example.com/status",
			expectError: false,
		},
		{
			name:     "variable in query string",
			template: "https://${API_HOST}/api?token=${API_TOKEN}&user=${USER_ID}",
			envVars: map[string]string{
				"API_HOST":  "api.example.com",
				"API_TOKEN": "abc123",
				"USER_ID":   "user456",
			},
			expected:    "https://api.example.com/api?token=abc123&user=user456",
			expectError: false,
		},
		{
			name:     "variable with empty value",
			template: "https://${API_HOST}/api/${EMPTY_VAR}/data",
			envVars: map[string]string{
				"API_HOST":  "api.example.com",
				"EMPTY_VAR": "",
			},
			expected:    "https://api.example.com/api//data",
			expectError: false,
		},
		{
			name:     "variable with spaces",
			template: "https://${API_HOST}/api/${USER_NAME}/profile",
			envVars: map[string]string{
				"API_HOST":  "api.example.com",
				"USER_NAME": "John Doe",
			},
			expected:    "https://api.example.com/api/John Doe/profile",
			expectError: false,
		},
		{
			name:     "complex path with variables",
			template: "https://${REGION}.${SERVICE}.${PROVIDER}.com/${VERSION}/${RESOURCE}/${ID}",
			envVars: map[string]string{
				"REGION":   "us-west-2",
				"SERVICE":  "compute",
				"PROVIDER": "aws",
				"VERSION":  "v1",
				"RESOURCE": "instances",
				"ID":       "i-1234567890abcdef0",
			},
			expected:    "https://us-west-2.compute.aws.com/v1/instances/i-1234567890abcdef0",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyURLTemplate(tt.template, tt.envVars)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestApplyURLTemplateEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		envVars     map[string]string
		description string
		expected    string
	}{
		{
			name:        "unmatched variable remains",
			template:    "https://${API_HOST}/api/${MISSING_VAR}/data",
			envVars:     map[string]string{"API_HOST": "api.example.com"},
			description: "Variables not in envVars should remain unchanged in the result",
			expected:    "https://api.example.com/api/${MISSING_VAR}/data",
		},
		{
			name:        "case sensitive variables",
			template:    "https://${API_HOST}/api/${api_host}/data",
			envVars:     map[string]string{"API_HOST": "api.example.com", "api_host": "different.example.com"},
			description: "Variable names are case sensitive",
			expected:    "https://api.example.com/api/different.example.com/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyURLTemplate(tt.template, tt.envVars)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestApplyURLTemplatePerformance(t *testing.T) {
	// Test with a large number of variables
	largeEnvVars := make(map[string]string, 1000)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("VAR_%d", i)
		value := fmt.Sprintf("value_%d", i)
		largeEnvVars[key] = value
	}

	template := "https://example.com/api"
	for i := 0; i < 100; i++ {
		template += fmt.Sprintf("/${VAR_%d}", i)
	}

	start := time.Now()
	result, err := applyURLTemplate(template, largeEnvVars)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	if result == "" {
		t.Errorf("expected non-empty result")
		return
	}

	// Performance should be reasonable (less than 100ms for 100 variables)
	if duration > 100*time.Millisecond {
		t.Errorf("performance test took too long: %v", duration)
	}

	t.Logf("Processed template with 100 variables in %v", duration)
}

func TestApplyURLTemplateRealWorldExamples(t *testing.T) {
	tests := []struct {
		name     string
		template string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "Databricks example",
			template: "https://${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}",
			envVars: map[string]string{
				"DATABRICKS_WORKSPACE_URL":  "workspace.cloud.databricks.com",
				"DATABRICKS_GENIE_SPACE_ID": "12345",
			},
			expected: "https://workspace.cloud.databricks.com/api/2.0/mcp/genie/12345",
		},
		{
			name:     "AWS API Gateway",
			template: "https://${API_ID}.execute-api.${REGION}.amazonaws.com/${STAGE}/${RESOURCE}",
			envVars: map[string]string{
				"API_ID":   "abc123def4",
				"REGION":   "us-east-1",
				"STAGE":    "prod",
				"RESOURCE": "users",
			},
			expected: "https://abc123def4.execute-api.us-east-1.amazonaws.com/prod/users",
		},
		{
			name:     "Google Cloud",
			template: "https://${PROJECT_ID}.${REGION}.run.app/${SERVICE_NAME}",
			envVars: map[string]string{
				"PROJECT_ID":   "my-project-123",
				"REGION":       "us-central1",
				"SERVICE_NAME": "api-service",
			},
			expected: "https://my-project-123.us-central1.run.app/api-service",
		},
		{
			name:     "Azure Functions",
			template: "https://${FUNCTION_APP}.azurewebsites.net/api/${FUNCTION_NAME}?code=${FUNCTION_KEY}",
			envVars: map[string]string{
				"FUNCTION_APP":  "my-function-app",
				"FUNCTION_NAME": "process-data",
				"FUNCTION_KEY":  "abc123def456",
			},
			expected: "https://my-function-app.azurewebsites.net/api/process-data?code=abc123def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyURLTemplate(tt.template, tt.envVars)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

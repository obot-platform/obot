package scim

import (
	"net/http"
	"strings"
)

// serviceProviderConfig describes what this endpoint implements: PATCH and filters, but no bulk operations,
// sorting, ETags, or password changes.
func serviceProviderConfig(baseURL string) map[string]any {
	return map[string]any{
		"schemas": []string{serviceProviderConfigSchema},
		"patch": map[string]any{
			"supported": true,
		},
		"bulk": map[string]any{
			"supported":      false,
			"maxOperations":  0,
			"maxPayloadSize": 0,
		},
		"filter": map[string]any{
			"supported":  true,
			"maxResults": maxCount,
		},
		"changePassword": map[string]any{
			"supported": false,
		},
		"sort": map[string]any{
			"supported": false,
		},
		"etag": map[string]any{
			"supported": false,
		},
		"authenticationSchemes": []map[string]any{
			{
				"type":        "oauthbearertoken",
				"name":        "Bearer token",
				"description": "A bearer token that Obot issued for this SCIM connection",
				"primary":     true,
			},
		},
		"meta": map[string]any{
			"resourceType": "ServiceProviderConfig",
			"location":     baseURL + "/ServiceProviderConfig",
		},
	}
}

func resourceTypes(baseURL string) []map[string]any {
	types := make([]map[string]any, 0, 2)
	for _, s := range []*resourceSchema{userResourceSchema, groupResourceSchema} {
		types = append(types, map[string]any{
			"schemas":     []string{resourceTypeSchema},
			"id":          s.Name,
			"name":        s.Name,
			"endpoint":    "/" + s.Name + "s",
			"description": s.Description,
			"schema":      s.ID,
			"meta": map[string]any{
				"resourceType": "ResourceType",
				"location":     baseURL + "/ResourceTypes/" + s.Name,
			},
		})
	}
	return types
}

func schemas(baseURL string) []map[string]any {
	schemas := make([]map[string]any, 0, 2)
	for _, s := range []*resourceSchema{userResourceSchema, groupResourceSchema} {
		schemas = append(schemas, map[string]any{
			"schemas":     []string{schemaSchema},
			"id":          s.ID,
			"name":        s.Name,
			"description": s.Description,
			"attributes":  s.Attributes,
			"meta": map[string]any{
				"resourceType": "Schema",
				"location":     baseURL + "/Schemas/" + s.ID,
			},
		})
	}
	return schemas
}

// serveDiscovery serves a discovery collection, or the member named by the one remaining path segment.
func serveDiscovery(w http.ResponseWriter, items []map[string]any, rest []string) {
	if len(rest) == 0 {
		resources := make([]any, 0, len(items))
		for _, item := range items {
			resources = append(resources, item)
		}
		writeList(w, resources, int64(len(resources)), 1)
		return
	}

	for _, item := range items {
		if id, _ := item["id"].(string); strings.EqualFold(id, rest[0]) {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeError(w, notFound("%q not found", rest[0]))
}

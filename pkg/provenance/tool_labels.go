package provenance

import "context"

// ToolLabelScope uses resource identity rather than a mutable display name.
func ToolLabelScope(resourceType, resourceID string) string {
	return resourceType + ":" + resourceID
}

// ReviewedToolCapabilities returns only an administrator-approved resource
// override. A pending proposal never changes enforcement.
func (e *Engine) ReviewedToolCapabilities(ctx context.Context, scope, tool string) (Labels, bool) {
	if scope == "" || e.loadRefs(ctx) != nil {
		return nil, false
	}
	e.refLock.Lock()
	defer e.refLock.Unlock()
	l, ok := e.toolLabels[toolLabelKey(scope, tool)]
	return l.Labels, ok && l.Source == "admin"
}

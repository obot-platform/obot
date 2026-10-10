package cleanup

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// UnassociatedMCPResources removes deployments and connections from before vMCP
// ownership was required. Cleanup handles missing parents for associated resources.
func UnassociatedMCPResources(req router.Request, _ router.Response) error {
	switch obj := req.Object.(type) {
	case *v1.MCPServer:
		if obj.Spec.VMCPID == "" && obj.Spec.VMCPInstanceID == "" {
			return req.Delete(obj)
		}
	case *v1.MCPServerInstance:
		if obj.Spec.VMCPInstanceID == "" {
			return req.Delete(obj)
		}
	}
	return nil
}

package runstates

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gatewayClient *gclient.Client
}

func NewHandler(gatewayClient *gclient.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) Migrate(req router.Request, _ router.Response) error {
	rs := req.Object.(*v1.RunState)
	var run v1.Run
	// Use an uncached get because the run might not be in the cache.
	if err := req.Get(untriggered.UncachedGet(&run), rs.Namespace, rs.Name); err == nil {
		// If the run exists, then create the run state in the gateway database
		if err := h.gatewayClient.CreateRunState(req.Ctx, &gtypes.RunState{
			Namespace:  req.Object.GetNamespace(),
			Name:       req.Object.GetName(),
			ThreadName: rs.Spec.ThreadName,
			Program:    rs.Spec.Program,
			ChatState:  rs.Spec.ChatState,
			CallFrame:  rs.Spec.CallFrame,
			Output:     rs.Spec.Output,
			Done:       rs.Spec.Done,
			Error:      rs.Spec.Error,
		}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	// If all is successful, then delete the run state from the Kubernetes database.
	return kclient.IgnoreNotFound(req.Delete(rs))
}

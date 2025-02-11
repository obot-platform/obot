package daemontrigger

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

var log = logger.Package()

type Handler struct {
	dispatcher *dispatcher.Dispatcher
}

func New(dispatcher *dispatcher.Dispatcher) *Handler {
	return &Handler{
		dispatcher: dispatcher,
	}
}

func (h *Handler) EnsureDaemonTriggerProvider(req router.Request, resp router.Response) error {
	daemonTrigger := req.Object.(*v1.DaemonTrigger)

	// TODO(njhale): Ensure the namespace is correct for this
	log.Warnf("EnsureDaemonTriggerProvider called for daemon trigger %s/%s", req.Namespace, daemonTrigger.Spec.Provider)
	//url, err := h.dispatcher.URLForDaemonTriggerProvider(req.Ctx, req.Namespace, daemonTrigger.Spec.Provider)
	//if err != nil {
	//	return err
	//}

	// TODO(njhale): Remove debug logging
	//log.Warnf("Daemon trigger provider running at: %q", url.String())

	return nil
}

//func (h *Handler) providerConfigured(req router.Request, providerNamespace, providerName) (bool, error) {
//	var toolRef v1.ToolReference
//	if err := req.Get(&toolRef, providerNamespace, providerName); apierrors.IsNotFound(err) {
//		// TODO(njhale): Decide what to do when we can't find the provider in general
//		// 	For now, just return and wait for the next update
//		return false, nil
//	} else if err != nil {
//		return false, err
//	}
//
//	// Determine if the provider is configured
//	// TODO(njhale): Encapsulate grabbing the tool ref and its configuration creds in a separate function.
//	//  We really ought to figure out how to _not_ make a ton of credential reveals here too.
//	dtps, err := providers.ConvertDaemonTriggerProviderToolRef(toolRef, nil)
//	if err != nil {
//		// TODO(njhale): Decide what to do when we can't convert the tool ref
//		return false, err
//	}
//
//	if len(dtps.RequiredConfigurationParameters) > 0 {
//		cred, err := h.gptClient.RevealCredential(req.Ctx, []string{string(toolRef.UID), system.GenericModelProviderCredentialContext}, toolRef.Name)
//		if err != nil {
//			// Not configured
//			return false, nil
//		}
//		dtps, err = providers.ConvertDaemonTriggerProviderToolRef(toolRef, cred.Env)
//		if err != nil {
//			return false, err
//		}
//	}
//
//	return dtps.Configured, nil
//}

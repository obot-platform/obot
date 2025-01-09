package toolinfo

import (
	"context"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/creds"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Handler struct {
	gptscript *gptscript.GPTScript
}

func New(gptscript *gptscript.GPTScript) *Handler {
	return &Handler{
		gptscript: gptscript,
	}
}

// SetToolInfoStatus will set the tool information for the object. This includes credential information,
// and whether those credentials exist.
// This handler should be used with the generationed.UpdateObservedGeneration to ensure that the processing
// is correctly reported to through the API.
func (h *Handler) SetToolInfoStatus(req router.Request, resp router.Response) (err error) {
	defer func() {
		if err != nil {
			resp.Attributes()["generation:errored"] = true
		}
	}()

	// Get all the credentials that exist in the expected context.
	creds, err := h.gptscript.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{req.Name, req.Namespace},
	})
	if err != nil {
		return err
	}

	credsSet := make(sets.Set[string], len(creds))
	for _, cred := range creds {
		credsSet.Insert(cred.ToolName)
	}

	obj := req.Object.(v1.ToolUser)
	tools := obj.GetTools()
	toolInfos := make(map[string]types.ToolInfo, len(tools))

	var (
		toolRef   v1.ToolReference
		credNames []string
	)
	for _, tool := range tools {
		if strings.ContainsAny(tool, "/.") {
			credNames, err = h.credentialNamesForNonToolReferences(req.Ctx, tool)
			if err != nil {
				return err
			}
		} else if err = req.Get(&toolRef, req.Namespace, tool); apierror.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		} else if toolRef.Status.Tool == nil {
			return fmt.Errorf("cannot determine credential status for tool %s: no tool status found", tool)
		} else if err == nil {
			credNames = toolRef.Status.Tool.CredentialNames
			// Clear the field we care about in this loop.
			// This allows us to use the same variable for the whole loop
			// while ensuring that the value we care about is loaded correctly.
			toolRef.Status.Tool.CredentialNames = nil
		}

		for i := 0; i < len(credNames); i++ {
			if credNames[i] == system.ModelProviderCredential {
				credNames = append(credNames[:i], credNames[i+1:]...)
				i--
			}
		}

		toolInfos[tool] = types.ToolInfo{
			CredentialNames: credNames,
			Authorized:      credsSet.HasAll(credNames...),
		}
	}

	obj.SetToolInfos(toolInfos)

	return nil
}

func (h *Handler) credentialNamesForNonToolReferences(ctx context.Context, name string) ([]string, error) {
	prg, err := h.gptscript.LoadFile(ctx, name)
	if err != nil {
		return nil, err
	}

	_, credNames, err := creds.DetermineCredsAndCredNames(prg, prg.ToolSet[prg.EntryToolID], name)
	return credNames, err
}

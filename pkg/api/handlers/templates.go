package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type TemplateHandler struct {
	cachedClient kclient.Client
}

func NewTemplateHandler(cachedClient kclient.Client) *TemplateHandler {
	return &TemplateHandler{
		cachedClient: cachedClient,
	}
}

func (t *TemplateHandler) GetTemplate(req api.Context) error {
	var (
		templates v1.ThreadTemplateList
		id        = req.PathValue("id")
	)
	if err := req.List(&templates, kclient.MatchingFields{
		"status.publicID": id,
	}); err != nil {
		return err
	}

	if len(templates.Items) == 0 {
		return types.NewErrNotFound("template %q not found", id)
	}

	return req.Write(convertThreadTemplate(templates.Items[0]))
}

func (t *TemplateHandler) ListTemplates(req api.Context) error {
	templates, err := listTemplates(req, t.cachedClient)
	if err != nil {
		return err
	}

	return req.Write(templates)
}

func listTemplates(req api.Context, cachedClient kclient.Client) (result types.ProjectTemplateList, _ error) {
	var (
		threads v1.ThreadTemplateList
		auths   v1.ThreadTemplateAuthorizationList
		seen    = make(map[string]bool)
	)

	err := req.List(&threads, kclient.MatchingFields{
		"spec.userID": req.User.GetUID(),
	})
	if err != nil {
		return result, err
	}

	for _, templates := range threads.Items {
		seen[templates.Name] = true
		result.Items = append(result.Items, convertThreadTemplate(templates))
	}

	var keys []string
	if email, ok := getEmail(req); ok {
		keys = []string{email, req.User.GetUID()}
	} else {
		keys = []string{req.User.GetUID()}
	}

	for _, key := range keys {
		err = req.List(&auths, kclient.MatchingFields{
			"spec.userID": key,
		})
		if err != nil {
			return result, err
		}

		for _, auth := range auths.Items {
			if seen[auth.Spec.TemplateID] {
				continue
			}
			var thread v1.ThreadTemplate
			if err := cachedClient.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: auth.Spec.TemplateID}, &thread); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return result, err
			}

			result.Items = append(result.Items, convertThreadTemplate(thread))
			seen[auth.Spec.TemplateID] = true
		}
	}

	return result, nil
}

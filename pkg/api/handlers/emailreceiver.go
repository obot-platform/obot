package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EmailReceiverHandler struct {
	hostname string
}

func NewEmailReceiverHandler(hostname string) *EmailReceiverHandler {
	return &EmailReceiverHandler{
		hostname: hostname,
	}
}

func (e *EmailReceiverHandler) Update(req api.Context) error {
	var (
		id = req.PathValue("id")
		er v1.EmailReceiver
	)

	if err := alias.Get(req.Context(), req.Storage, &er, req.Namespace(), id); err != nil {
		return err
	}

	var manifest types.EmailReceiverManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	er.Spec.EmailReceiverManifest = manifest
	if err := req.Update(&er); err != nil {
		return err
	}

	return req.Write(convertEmailReceiver(er, e.hostname))
}

func (e *EmailReceiverHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	return req.Delete(&v1.EmailReceiver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (e *EmailReceiverHandler) Create(req api.Context) error {
	var manifest types.EmailReceiverManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	er := &v1.EmailReceiver{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.EmailReceiverPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.EmailReceiverSpec{
			EmailReceiverManifest: manifest,
		},
	}

	if err := req.Create(er); err != nil {
		return err
	}

	return req.WriteCreated(convertEmailReceiver(*er, e.hostname))
}

func convertEmailReceiver(emailReceiver v1.EmailReceiver, hostname string) *types.EmailReceiver {
	manifest := emailReceiver.Spec.EmailReceiverManifest

	var aliasAssigned *bool
	if emailReceiver.Generation == emailReceiver.Status.ObservedGeneration {
		aliasAssigned = &emailReceiver.Status.AliasAssigned
	}
	er := &types.EmailReceiver{
		Metadata:              MetadataFrom(&emailReceiver),
		EmailReceiverManifest: manifest,
		AliasAssigned:         aliasAssigned,
	}

	if hostname != "" {
		name := emailReceiver.Name
		// If alias Name is set, we should return the alias email address if AliasAssigned is true, otherwise return the original email address
		// return empty email address if AliasAssigned is not set because UI will need to poll until AliasAssigned is set
		if emailReceiver.Spec.Alias != "" {
			if er.AliasAssigned == nil {
				er.EmailAddress = ""
			} else if *er.AliasAssigned {
				er.EmailAddress = fmt.Sprintf("%s@%s", emailReceiver.Spec.Alias, hostname)
			} else {
				er.EmailAddress = fmt.Sprintf("%s@%s", name, hostname)
			}
		} else {
			er.EmailAddress = fmt.Sprintf("%s@%s", name, hostname)
		}
	}
	return er
}

func (e *EmailReceiverHandler) ByID(req api.Context) error {
	var (
		er v1.EmailReceiver
		id = req.PathValue("id")
	)

	if err := alias.Get(req.Context(), req.Storage, &er, req.Namespace(), id); err != nil {
		return err
	}

	return req.Write(convertEmailReceiver(er, e.hostname))
}

func (e *EmailReceiverHandler) List(req api.Context) error {
	var emailReceiverList v1.EmailReceiverList
	if err := req.List(&emailReceiverList); err != nil {
		return err
	}

	var resp types.EmailReceiverList
	for _, er := range emailReceiverList.Items {
		resp.Items = append(resp.Items, *convertEmailReceiver(er, e.hostname))
	}

	return req.Write(resp)
}

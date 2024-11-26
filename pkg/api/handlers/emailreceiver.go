package handlers

import (
	"fmt"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/alias"
	"github.com/otto8-ai/otto8/pkg/api"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/wait"
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

	processedEr, err := wait.For(req.Context(), req.Storage, &er, func(er *v1.EmailReceiver) bool {
		return er.Generation == er.Status.AliasObservedGeneration
	})
	if err != nil {
		return fmt.Errorf("failed to update email receiver: %w", err)
	}

	return req.Write(convertEmailReceiver(*processedEr, e.hostname))
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

	er, err := wait.For(req.Context(), req.Storage, er, func(er *v1.EmailReceiver) bool {
		return er.Generation == er.Status.AliasObservedGeneration
	}, wait.Option{Create: true})
	if err != nil {
		return fmt.Errorf("failed to create email receiver: %w", err)
	}

	return req.WriteCreated(convertEmailReceiver(*er, e.hostname))
}

func convertEmailReceiver(emailReceiver v1.EmailReceiver, hostname string) *types.EmailReceiver {
	manifest := emailReceiver.Spec.EmailReceiverManifest
	er := &types.EmailReceiver{
		Metadata:              MetadataFrom(&emailReceiver),
		EmailReceiverManifest: manifest,
		AddressAssigned:       emailReceiver.Status.AliasAssigned,
	}
	if hostname != "" && er.AddressAssigned {
		er.EmailAddress = emailReceiver.Spec.User + "@" + hostname
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

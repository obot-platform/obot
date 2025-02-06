package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DaemonTriggerHandler struct{}

func (e *DaemonTriggerHandler) Update(req api.Context) error {
	var (
		id = req.PathValue("id")
		dt v1.DaemonTrigger
	)

  if err := req.Get(&dt, id); err != nil {
    return err
  }

	var manifest types.DaemonTriggerManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

  // TODO(njhale): Fail Update when provider doesn't exist

	dt.Spec.DaemonTriggerManifest = manifest
	if err := req.Update(&dt); err != nil {
		return err
	}

	return req.Write(convertDaemonTrigger(dt))
}

func (*DaemonTriggerHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	return req.Delete(&v1.DaemonTrigger{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (*DaemonTriggerHandler) Create(req api.Context) error {
	var manifest types.DaemonTriggerManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	dt := &v1.DaemonTrigger{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.DaemonTriggerPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.DaemonTriggerSpec{
			DaemonTriggerManifest: manifest,
		},
	}

  // TODO(njhale): Fail Create when provider doesn't exist

	if err := req.Create(dt); err != nil {
		return err
	}

	return req.WriteCreated(convertDaemonTrigger(*dt))
}

func convertDaemonTrigger(DaemonTrigger v1.DaemonTrigger) *types.DaemonTrigger {
	manifest := DaemonTrigger.Spec.DaemonTriggerManifest
	er := &types.DaemonTrigger{
		Metadata:              MetadataFrom(&DaemonTrigger),
		DaemonTriggerManifest: manifest,
	}

	return er
}

func (e *DaemonTriggerHandler) ByID(req api.Context) error {
	var (
		dt v1.DaemonTrigger
		id = req.PathValue("id")
	)

  if err := req.Get(&dt, id); err != nil {
    return err
  }


	return req.Write(convertDaemonTrigger(dt))
}

func (*DaemonTriggerHandler) List(req api.Context) error {
	var daemonTriggers v1.DaemonTriggerList
	if err := req.List(&daemonTriggers); err != nil {
		return err
	}

	var resp types.DaemonTriggerList
	for _, dt := range daemonTriggers.Items {
		resp.Items = append(resp.Items, *convertDaemonTrigger(dt))
	}

	return req.Write(resp)
}

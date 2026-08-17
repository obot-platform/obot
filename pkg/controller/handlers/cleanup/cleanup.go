package cleanup

import (
	"fmt"
	"log/slog"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func Cleanup(req router.Request, _ router.Response) error {
	toDelete := req.Object.(v1.DeleteRefs)

	for _, ref := range toDelete.DeleteRefs() {
		if ref.Name == "" {
			continue
		}

		namespace := req.Namespace
		if namespace == "" && ref.Namespace != "" {
			namespace = ref.Namespace
		}

		objType := ref.ObjType
		if ref.Kind != "" {
			o, err := req.Client.Scheme().New(schema.GroupVersionKind{
				Group:   objType.GetObjectKind().GroupVersionKind().Group,
				Version: objType.GetObjectKind().GroupVersionKind().Version,
				Kind:    ref.Kind,
			})
			if err != nil {
				return err
			}
			objType = o.(kclient.Object)
		}

		if ref.Alias != "" {
			var alias v1.Alias
			if err := req.Get(&alias, namespace, ref.Alias); !apierrors.IsNotFound(err) {
				return err
			}
			if err := req.Get(untriggered.UncachedGet(&alias), namespace, ref.Alias); !apierrors.IsNotFound(err) {
				return err
			}
		}

		if err := req.Get(objType, namespace, ref.Name); apierrors.IsNotFound(err) {
			if err := req.Get(untriggered.UncachedGet(objType), namespace, ref.Name); apierrors.IsNotFound(err) {
				slog.Info("Deleting object due to missing reference",
					"gvk", req.GVK.String(), "namespace", namespace, "name", req.Name,
					"refType", fmt.Sprintf("%T", objType), "refName", ref.Name)
				return req.Delete(req.Object)
			}
		}
	}

	return nil
}

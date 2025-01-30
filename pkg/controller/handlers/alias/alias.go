package alias

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/create"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

func matches(alias *v1.Alias, obj kclient.Object) bool {
	return alias.Spec.TargetName == obj.GetName() &&
		alias.Spec.TargetNamespace == obj.GetNamespace() &&
		alias.Spec.TargetKind == obj.GetObjectKind().GroupVersionKind().Kind
}

// AssignAlias will check the requested alias to see if it is already assigned to another object.
// If it is not, then the alias is assigned to the currently processing object.
// This handler should be used with the generationed.UpdateObservedGeneration to ensure that the processing
// is correctly reported to through the API.
func AssignAlias(req router.Request, resp router.Response) (err error) {
	defer func() {
		if err != nil {
			resp.Attributes()["generation:errored"] = true
		}
	}()

	aliasable := req.Object.(v1.Aliasable)

	if aliasable.GetAliasName() == "" {
		if aliasable.IsAssigned() || aliasable.GetGeneration() != aliasable.GetObservedGeneration() {
			aliasable.SetAssigned(false)
			return req.Client.Status().Update(req.Ctx, req.Object)
		}

		return nil
	}

	gvk, err := req.Client.GroupVersionKindFor(req.Object)
	if err != nil {
		return err
	}

	key, err := alias.Name(alias.FromGVK(gvk), aliasable)
	if err != nil {
		return err
	}

	alias := &v1.Alias{
		ObjectMeta: metav1.ObjectMeta{
			Name: key,
		},
		Spec: v1.AliasSpec{
			Name:            aliasable.GetAliasName(),
			TargetName:      req.Object.GetName(),
			TargetNamespace: req.Object.GetNamespace(),
			TargetKind:      gvk.Kind,
		},
	}
	if err = create.IfNotExists(req.Ctx, req.Client, alias); err != nil {
		return err
	}

	if assigned := matches(alias, req.Object); assigned != aliasable.IsAssigned() {
		aliasable.SetAssigned(assigned)
		return req.Client.Status().Update(req.Ctx, req.Object)
	}

	return nil
}

func UnassignAlias(req router.Request, _ router.Response) error {
	src := req.Object.(*v1.Alias)
	if src.Spec.TargetName == "" || src.Spec.TargetKind == "" {
		return fmt.Errorf("invalid alias %s, missing kind=%s or name=%s", src.Name, src.Spec.TargetKind, src.Spec.TargetName)
	}

	gvk := schema.GroupVersionKind{
		Group:   v1.SchemeGroupVersion.Group,
		Version: v1.SchemeGroupVersion.Version,
		Kind:    src.Spec.TargetKind,
	}

	target, err := req.Client.Scheme().New(gvk)
	if err != nil {
		return err
	}

	aliasable, ok := target.(v1.Aliasable)
	if !ok {
		return fmt.Errorf("object %s does not support aliasing, invalid alias %s", src.Spec.TargetKind, src.Name)
	}

	// First check happy path, because this is the fastest and most common
	if err := req.Get(target.(kclient.Object), src.Spec.TargetNamespace, src.Spec.TargetName); err == nil {
		if aliasName, err := alias.Name(req.Client, aliasable); err == nil && aliasName == src.Name {
			// In sync, all good
			return nil
		}
	}

	// Happy path failed, grab the target object uncached
	if err := req.Get(untriggered.UncachedGet(target.(kclient.Object)), src.Spec.TargetNamespace, src.Spec.TargetName); err != nil {
		if apierrors.IsNotFound(err) {
			// Target object does not exist, delete alias
			log.Infof("Target object %s/%s does not exist, deleting alias %s", src.Spec.TargetNamespace, src.Spec.TargetName, src.Name)
			return req.Delete(src)
		}
		return err
	}

	// Check if alias name algorithm has changed
	if src.Name != alias.KeyFromScopeID(alias.GetScope(gvk, aliasable), src.Spec.Name) {
		log.Infof("Alias name algorithm has changed, deleting alias %s", src.Name)
		return req.Delete(src)
	}

	aliasName, err := alias.Name(req.Client, aliasable)
	if err != nil {
		return err
	}

	if aliasName != src.Name {
		// Alias name does not match, delete alias
		log.Infof("Alias name %q does not match expected %q, deleting alias %q", src.Name, aliasName, src.Name)
		return req.Delete(src)
	}

	return nil
}

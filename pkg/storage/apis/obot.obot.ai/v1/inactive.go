package v1

import kclient "sigs.k8s.io/controller-runtime/pkg/client"

const LabelInactive = "obot_inactive_object"

func SetInactive(o kclient.Object) {
	if o.GetLabels() == nil {
		o.SetLabels(make(map[string]string, 1))
	}
	o.GetLabels()[LabelInactive] = "true"
}

func SetActive(o kclient.Object) {
	delete(o.GetLabels(), LabelInactive)
}

func IsActive(o kclient.Object) bool {
	return o.GetLabels()[LabelInactive] != "true"
}

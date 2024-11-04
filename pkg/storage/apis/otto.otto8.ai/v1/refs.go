package v1

import kclient "sigs.k8s.io/controller-runtime/pkg/client"

// +k8s:deepcopy-gen=false

type Ref struct {
	ObjType   kclient.Object
	Namespace string
	Name      string
}

package v1

// +k8s:deepcopy-gen=false

type Generationed interface {
	GetObservedGeneration() int64
	SetObservedGeneration(int64)
}

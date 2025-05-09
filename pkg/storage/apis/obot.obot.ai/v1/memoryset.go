package v1

import (
	"math/big"

	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MemorySet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemorySetSpec   `json:"spec,omitempty"`
	Status MemorySetStatus `json:"status,omitempty"`
}

type MemorySetSpec struct {
	// ThreadName is the name of the thread that the MemorySet belongs to.
	ThreadName string `json:"threadName,omitempty"`

	// NextMemoryID is a string representation of a monotonically increasing positive integer used
	// to generate unique IDs for new memories.
	// This field MUST be used as the ID of the next memory added to the MemorySet.
	// This field MUST be incremented by 1 when a new memory is added to the MemorySet.
	NextMemoryID string `json:"nextMemoryID,omitempty"`

	// Memories contains the list of memories in the MemorySet.
	Memories []types.Memory `json:"memories,omitempty"`
}

func (in *MemorySet) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
	}
}

// GetNextMemoryID computes the next memory ID to use.
func (in MemorySet) GetNextMemoryID() *big.Int {
	if nextID, ok := new(big.Int).SetString(in.Spec.NextMemoryID, 10); ok && nextID != nil {
		// NextMemoryID is a valid integer, use it as the next memory ID
		return nextID.Abs(nextID)
	}

	// Assume NextMemoryID hasn't been initialized or is invalid, try to find the last valid ID in use
	var maxID *big.Int
	for _, m := range in.Spec.Memories {
		if m.ID == "" {
			continue
		}

		if id, ok := new(big.Int).SetString(m.ID, 10); ok && id != nil {
			id = id.Abs(id)
			if maxID == nil || id.Cmp(maxID) > 0 {
				maxID = id
			}
		}
	}

	if maxID == nil {
		// No valid IDs found, return 0
		return big.NewInt(0)
	}

	// Increment the largest valid ID to get the next memory ID
	return maxID.Add(maxID, big.NewInt(1))
}

type MemorySetStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MemorySetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MemorySet `json:"items"`
}

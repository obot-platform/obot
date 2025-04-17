package handlers

import (
	"slices"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type MemorySetHandler struct {
}

func NewMemorySetHandler() *MemorySetHandler {
	return &MemorySetHandler{}
}

func (*MemorySetHandler) AddMemories(req api.Context) error {
	var (
		memorySet v1.MemorySet
		memories  []types.Memory
	)

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Read(&memories); err != nil {
		return err
	}

	// Current time for new memories
	currentTime := types.NewTime(time.Now())

	// Assign IDs based on content hash and deduplicate
	newMemories := make(map[string]types.Memory, len(memories))
	for i := range memories {
		memory := memories[i]
		// Generate ID from content hash if not provided
		if memory.ID == "" {
			// Create a shorter URL-friendly hash (first 12 chars of SHA-256)
			fullHash := hash.String(memory.Content)
			memory.ID = fullHash[:12]
		}
		// Always set creation time to current time
		memory.CreatedAt = *currentTime
		newMemories[memory.ID] = memory
	}

	if err := req.Get(&memorySet, thread.Name); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		// Create a new MemorySet
		memorySet = v1.MemorySet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      thread.Name,
				Namespace: req.Namespace(),
			},
			Spec: v1.MemorySetSpec{
				ThreadName: thread.Name,
				Manifest: types.MemorySetManifest{
					Memories: []types.Memory{},
				},
			},
		}

		for _, memory := range newMemories {
			memorySet.Spec.Manifest.Memories = append(memorySet.Spec.Manifest.Memories, memory)
		}

		if err := req.Create(&memorySet); err != nil {
			return err
		}
	} else {
		// Add memories to the existing MemorySet
		existingMemories := make(map[string]struct{}, len(memorySet.Spec.Manifest.Memories))
		for _, memory := range memorySet.Spec.Manifest.Memories {
			existingMemories[memory.ID] = struct{}{}
		}

		for id, memory := range newMemories {
			if _, exists := existingMemories[id]; exists {
				continue
			}

			memorySet.Spec.Manifest.Memories = append(memorySet.Spec.Manifest.Memories, memory)
		}

		if err := req.Update(&memorySet); err != nil {
			return err
		}
	}

	return req.Write(&types.MemorySet{
		Metadata:          MetadataFrom(&memorySet),
		MemorySetManifest: memorySet.Spec.Manifest,
	})
}

func (*MemorySetHandler) GetMemories(req api.Context) error {
	var memorySet v1.MemorySet
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Get(&memorySet, thread.Name); err != nil {
		return err
	}

	return req.Write(&types.MemorySet{
		Metadata:          MetadataFrom(&memorySet),
		MemorySetManifest: memorySet.Spec.Manifest,
	})
}

func (*MemorySetHandler) DeleteMemories(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	// Check if a specific memory_id is provided
	memoryID := req.PathValue("memory_id")
	if memoryID == "" {
		// No memory_id provided, delete the entire memory set
		return req.Delete(&v1.MemorySet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      thread.Name,
				Namespace: req.Namespace(),
			},
		})
	}

	// Memory ID provided, delete just that specific memory
	var memorySet v1.MemorySet
	if err := req.Get(&memorySet, thread.Name); err != nil {
		return err
	}

	// Store original length to check if any memory was deleted
	numMemories := len(memorySet.Spec.Manifest.Memories)
	memorySet.Spec.Manifest.Memories = slices.DeleteFunc(memorySet.Spec.Manifest.Memories, func(memory types.Memory) bool {
		return memory.ID == memoryID
	})

	// Check if any memory was deleted
	if len(memorySet.Spec.Manifest.Memories) == numMemories {
		return apierrors.NewNotFound(schema.GroupResource{}, memoryID)
	}

	// Update the memory set with the filtered memories
	return req.Update(&memorySet)
}

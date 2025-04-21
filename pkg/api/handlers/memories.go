package handlers

import (
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type MemoryHandler struct {
}

func NewMemoryHandler() *MemoryHandler {
	return &MemoryHandler{}
}

func (*MemoryHandler) CreateMemory(req api.Context) error {
	var (
		memorySet v1.MemorySet
		memory    types.Memory
	)

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Read(&memory); err != nil {
		return err
	}

	// Return error if content is empty
	if memory.Content == "" {
		return apierrors.NewBadRequest("content cannot be empty")
	}

	// Return error if ID or timestamp is provided
	if memory.ID != "" {
		return apierrors.NewBadRequest("id should not be provided")
	}

	if memory.CreatedAt != (types.Time{}) {
		return apierrors.NewBadRequest("createdAt should not be provided")
	}

	// Set ID and timestamp
	memory.ID = uuid.NewString()
	memory.CreatedAt = *types.NewTime(time.Now())

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
				Memories:   []types.Memory{memory},
			},
		}

		if err := req.Create(&memorySet); err != nil {
			return err
		}
	} else {
		// Check if memory with same content already exists
		for _, existingMemory := range memorySet.Spec.Memories {
			if existingMemory.Content == memory.Content {
				// Memory with same content already exists, return existing memory
				return req.Write(&existingMemory)
			}
		}

		// Add the memory to the existing MemorySet
		memorySet.Spec.Memories = append(memorySet.Spec.Memories, memory)

		if err := req.Update(&memorySet); err != nil {
			return err
		}
	}

	// Return just the created memory
	return req.Write(&memory)
}

func (*MemoryHandler) ListMemories(req api.Context) error {
	var memorySet v1.MemorySet
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Get(&memorySet, thread.Name); err != nil {
		if apierrors.IsNotFound(err) {
			// Return empty list if no memories exist yet
			return req.Write(&types.MemoryList{
				Items: []types.Memory{},
			})
		}
		return err
	}

	// Return list of memories
	return req.Write(&types.MemoryList{
		Items: memorySet.Spec.Memories,
	})
}

func (*MemoryHandler) DeleteMemories(req api.Context) error {
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
	numMemories := len(memorySet.Spec.Memories)
	memorySet.Spec.Memories = slices.DeleteFunc(memorySet.Spec.Memories, func(memory types.Memory) bool {
		return memory.ID == memoryID
	})

	// Check if any memory was deleted
	if len(memorySet.Spec.Memories) == numMemories {
		return apierrors.NewNotFound(schema.GroupResource{}, memoryID)
	}

	// Update the memory set with the filtered memories
	return req.Update(&memorySet)
}

func (*MemoryHandler) UpdateMemory(req api.Context) error {
	var input struct {
		Content string `json:"content"`
	}

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	// Get memory_id from the path
	memoryID := req.PathValue("memory_id")
	if memoryID == "" {
		return apierrors.NewBadRequest("memory_id is required")
	}

	if err := req.Read(&input); err != nil {
		return err
	}

	// Return error if content is empty
	if input.Content == "" {
		return apierrors.NewBadRequest("memory content cannot be empty")
	}

	// Get the memory set
	var memorySet v1.MemorySet
	if err := req.Get(&memorySet, thread.Name); err != nil {
		return err
	}

	// Find and update the memory by ID
	var (
		updatedMemory types.Memory
		memoryFound   bool
	)
	for i, memory := range memorySet.Spec.Memories {
		if memory.ID == memoryID {
			memorySet.Spec.Memories[i].Content = input.Content
			updatedMemory = memorySet.Spec.Memories[i]
			memoryFound = true
			break
		}
	}

	if !memoryFound {
		return apierrors.NewNotFound(schema.GroupResource{}, memoryID)
	}

	// Update the memory set
	if err := req.Update(&memorySet); err != nil {
		return err
	}

	// Return just the updated memory
	return req.Write(&updatedMemory)
}

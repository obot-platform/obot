package handlers

import (
	"math/big"
	"slices"
	"strconv"
	"time"

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

// CreateMemory creates a new memory and responds with the created memory.
// If a memory with duplicate content already exists, this function no-ops and responds with the
// existing memory.
func (*MemoryHandler) CreateMemory(req api.Context) error {
	var memory types.Memory
	if err := req.Read(&memory); err != nil {
		return err
	}

	if memory.ID != "" {
		return apierrors.NewBadRequest("new memories must not contain an ID")
	}

	if unquoted, err := strconv.Unquote(memory.Content); err == nil {
		memory.Content = unquoted
	}

	if memory.Content == "" {
		return apierrors.NewBadRequest("content cannot be empty")
	}

	if memory.CreatedAt == nil || memory.CreatedAt.IsZero() {
		memory.CreatedAt = types.NewTime(time.Now())
	}

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var memorySet v1.MemorySet
	if err := req.Get(&memorySet, thread.Name); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		// MemorySet does not exist, create a new one with the given memory
		memory.ID = "0"
		memorySet = v1.MemorySet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      thread.Name,
				Namespace: req.Namespace(),
			},
			Spec: v1.MemorySetSpec{
				ThreadName:   thread.Name,
				Memories:     []types.Memory{memory},
				NextMemoryID: "1",
			},
		}

		if err := req.Create(&memorySet); err != nil {
			return err
		}

		return req.Write(&memory)
	}

	if memoryIndex := slices.IndexFunc(memorySet.Spec.Memories, func(m types.Memory) bool {
		return m.Content == memory.Content
	}); memoryIndex >= 0 {
		// A memory with matching content already exists
		// Respond with the existing memory from the MemorySet to preserve the CreatedAt timestamp
		return req.Write(&memorySet.Spec.Memories[memoryIndex])
	}

	// Set the memory ID and next memory ID in the MemorySet
	nextID := memorySet.GetNextMemoryID()
	memory.ID = nextID.String()
	memorySet.Spec.NextMemoryID = nextID.Add(nextID, big.NewInt(1)).String()

	// Memory with matching content doesn't exist, add it and update the MemorySet
	memorySet.Spec.Memories = append(memorySet.Spec.Memories, memory)
	if err := req.Update(&memorySet); err != nil {
		return err
	}

	return req.Write(&memory)
}

// ListMemories responds with a list containing all memories.
func (*MemoryHandler) ListMemories(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var memorySet v1.MemorySet
	if err := req.Get(&memorySet, thread.Name); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return req.Write(&types.MemoryList{
		Items: memorySet.Spec.Memories,
	})
}

// DeleteMemories deletes one or all memories and responds with the list of memories that were deleted.
// If the memory_id path parameter is provided, the memory with that ID is deleted.
// If no memory_id is provided, all memories are deleted.
func (*MemoryHandler) DeleteMemories(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var memorySet v1.MemorySet
	if err := req.Get(&memorySet, thread.Name); err != nil {
		return err
	}

	memoryID := req.PathValue("memory_id")
	if memoryID == "" {
		// Delete all memories by deleting the MemorySet
		if err := req.Delete(&memorySet); err != nil {
			return err
		}

		return req.Write(&types.MemoryList{
			Items: memorySet.Spec.Memories,
		})
	}

	// Delete any memories with the specified ID (cleanup any duplicates)
	deletedMemories := []types.Memory{}
	memorySet.Spec.Memories = slices.DeleteFunc(memorySet.Spec.Memories, func(m types.Memory) bool {
		if m.ID == memoryID {
			deletedMemories = append(deletedMemories, m)
			return true
		}

		return false
	})

	if len(deletedMemories) == 0 {
		return apierrors.NewNotFound(schema.GroupResource{}, memoryID)
	}

	if err := req.Update(&memorySet); err != nil {
		return err
	}

	return req.Write(&types.MemoryList{
		Items: deletedMemories,
	})
}

// UpdateMemory updates the memory with the specified ID and responds with the updated memory.
// If a memory with the same content already exists, this function no-ops and responds with the existing
// memory.
func (*MemoryHandler) UpdateMemory(req api.Context) error {
	memoryID := req.PathValue("memory_id")
	if memoryID == "" {
		return apierrors.NewBadRequest("memory_id is required")
	}

	var memory types.Memory
	if err := req.Read(&memory); err != nil {
		return err
	}

	if unquoted, err := strconv.Unquote(memory.Content); err == nil {
		memory.Content = unquoted
	}

	if memory.Content == "" {
		return apierrors.NewBadRequest("memory content cannot be empty")
	}

	if memory.CreatedAt == nil || memory.CreatedAt.IsZero() {
		memory.CreatedAt = types.NewTime(time.Now())
	}

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var memorySet v1.MemorySet
	if err := req.Get(&memorySet, thread.Name); err != nil {
		return err
	}

	memoryIndex := slices.IndexFunc(memorySet.Spec.Memories, func(m types.Memory) bool {
		return m.ID == memoryID
	})
	if memoryIndex < 0 {
		return apierrors.NewNotFound(schema.GroupResource{}, memoryID)
	}

	existingMemory := memorySet.Spec.Memories[memoryIndex]
	if existingMemory.Content == memory.Content {
		// No change, respond with the existing memory
		return req.Write(&existingMemory)
	}

	// Memory content changed, update the MemorySet and respond with the updated memory
	memory.ID = memoryID
	memorySet.Spec.Memories[memoryIndex] = memory
	if err := req.Update(&memorySet); err != nil {
		return err
	}

	return req.Write(&memory)
}

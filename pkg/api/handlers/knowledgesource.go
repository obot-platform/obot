package handlers

import (
	"bytes"
	"encoding/json"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/gz"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
)

func convertKnowledgeSource(agentName string, knowledgeSource v1.KnowledgeSource) types.KnowledgeSource {
	var syncDetails []byte
	if len(knowledgeSource.Status.SyncDetails) > 0 {
		_ = gz.Decompress(&syncDetails, knowledgeSource.Status.SyncDetails)
	}
	return types.KnowledgeSource{
		Metadata:                MetadataFrom(&knowledgeSource),
		KnowledgeSourceManifest: knowledgeSource.Spec.Manifest,
		AgentID:                 agentName,
		State:                   knowledgeSource.PublicState(),
		SyncDetails:             syncDetails,
		Status:                  knowledgeSource.Status.Status,
		Error:                   knowledgeSource.Status.Error,
		LastSyncStartTime:       types.NewTime(knowledgeSource.Status.LastSyncStartTime.Time),
		LastSyncEndTime:         types.NewTime(knowledgeSource.Status.LastSyncEndTime.Time),
		LastRunID:               knowledgeSource.Status.RunName,
		FilePathPrefixExclude:   knowledgeSource.Spec.Manifest.FilePathPrefixExclude,
		FilePathPrefixInclude:   knowledgeSource.Spec.Manifest.FilePathPrefixInclude,
	}
}

func checkConfigChanged(oldValue, newValue types.KnowledgeSourceInput) bool {
	oldData, _ := json.Marshal(oldValue)
	newData, _ := json.Marshal(newValue)
	return !bytes.Equal(oldData, newData)
}

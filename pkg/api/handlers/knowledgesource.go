package handlers

import (
	"bytes"
	"encoding/json"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/gz"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
)

func convertRemoteKnowledgeSource(agentName string, knowledgeSource v1.KnowledgeSource) types.KnowledgeSource {
	var syncDetails []byte
	if len(knowledgeSource.Status.SyncState) > 0 {
		_ = gz.Decompress(syncDetails, knowledgeSource.Status.SyncDetails)
	}
	return types.KnowledgeSource{
		Metadata:                MetadataFrom(&knowledgeSource),
		KnowledgeSourceManifest: knowledgeSource.Spec.Manifest,
		AgentID:                 agentName,
		State:                   knowledgeSource.PublicState(),
		SyncDetails:             syncDetails,
		Status:                  knowledgeSource.Status.Status,
		Error:                   knowledgeSource.Status.Error,
		AuthStatus:              knowledgeSource.Status.Auth,
	}
}

func checkConfigChanged(oldValue, newValue types.KnowledgeSourceInput) bool {
	oldData, _ := json.Marshal(oldValue)
	newData, _ := json.Marshal(newValue)
	return !bytes.Equal(oldData, newData)
}

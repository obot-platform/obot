package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NanobotHandler struct{}

func NewNanobotHandler() *NanobotHandler {
	return &NanobotHandler{}
}

func (n *NanobotHandler) List(req api.Context) error {
	var list v1.NanobotConfigList
	if err := req.List(&list); err != nil {
		return err
	}

	items := make([]types.NanobotConfig, 0, len(list.Items))
	for _, config := range list.Items {
		items = append(items, convertNanobotConfig(config))
	}

	return req.Write(types.NanobotConfigList{Items: items})
}

func (n *NanobotHandler) Get(req api.Context) error {
	var config v1.NanobotConfig
	if err := req.Get(&config, req.PathValue("nanobot_config_id")); err != nil {
		return err
	}

	return req.Write(convertNanobotConfig(config))
}

func (n *NanobotHandler) Create(req api.Context) error {
	var input types.NanobotConfig
	if err := req.Read(&input); err != nil {
		return err
	}

	config := &v1.NanobotConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    req.Namespace(),
			GenerateName: system.NanobotConfigPrefix,
		},
		Spec: v1.NanobotConfigSpec{
			Manifest: input.NanobotConfigManifest,
			UserID:   req.User.GetUID(),
		},
	}

	if err := req.Create(config); err != nil {
		return err
	}

	return req.Write(convertNanobotConfig(*config))
}

func (n *NanobotHandler) Update(req api.Context) error {
	var (
		config v1.NanobotConfig
		input  types.NanobotConfig
	)
	if err := req.Get(&config, req.PathValue("nanobot_config_id")); err != nil {
		return err
	}

	if err := req.Read(&input); err != nil {
		return err
	}

	config.Spec.Manifest = input.NanobotConfigManifest

	if err := req.Update(&config); err != nil {
		return err
	}

	return req.Write(convertNanobotConfig(config))
}

func (n *NanobotHandler) Delete(req api.Context) error {
	var config v1.NanobotConfig
	if err := req.Get(&config, req.PathValue("nanobot_config_id")); err != nil {
		return err
	}

	return req.Delete(&config)
}

func convertNanobotConfig(config v1.NanobotConfig) types.NanobotConfig {
	return types.NanobotConfig{
		Metadata:              MetadataFrom(&config),
		NanobotConfigManifest: config.Spec.Manifest,
	}
}

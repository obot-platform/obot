package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mdmassets"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"gorm.io/gorm"
)

// MDMConfigurationsHandler serves the admin API for MDM configurations and
// their enrollment keys.
type MDMConfigurationsHandler struct {
	serverURL string
}

func NewMDMConfigurationsHandler(serverURL string) *MDMConfigurationsHandler {
	return &MDMConfigurationsHandler{serverURL: serverURL}
}

// Create validates and persists a configuration, applying the latest asset to
// blank requests when its default values are valid.
func (h *MDMConfigurationsHandler) Create(req api.Context) error {
	var in types.MDMConfiguration
	if err := req.Read(&in); err != nil {
		return err
	}

	// Resolve the enforcement policy up front so malformed input is rejected
	// before any asset work; it is applied to the final configuration below.
	allowlist, err := enforcementAllowlistForSave(in.EnforcementEnabled, in.EnforcementAllowlist, nil)
	if err != nil {
		return err
	}

	configuration, err := h.mdmConfigurationFromInput(req, in, nil)
	if err != nil {
		return err
	}

	if configuration.AssetDigest == "" {
		if source, err := getMDMAssetSource(req); err == nil &&
			source.Annotations[v1.MDMAssetSourceSyncAnnotation] != "true" &&
			source.Status.LatestDigest != "" {
			auto, err := h.mdmConfigurationFromInput(req, types.MDMConfiguration{
				MDMConfigurationManifest: types.MDMConfigurationManifest{
					AssetDigest: source.Status.LatestDigest,
				},
			}, nil)
			if err == nil {
				configuration = auto
			}
		}
	}

	configuration.EnforcementEnabled = in.EnforcementEnabled
	configuration.EnforcementAllowlist = allowlist

	created, err := req.GatewayClient.CreateMDMConfiguration(req.Context(), req.UserID(), configuration)
	if err != nil {
		return err
	}
	result, err := convertMDMConfiguration(*created)
	if err != nil {
		return err
	}
	return req.WriteCreated(result)
}

// List handles GET /api/mdm/configurations.
func (h *MDMConfigurationsHandler) List(req api.Context) error {
	configurations, err := req.GatewayClient.ListMDMConfigurations(req.Context())
	if err != nil {
		return err
	}
	items := make([]types.MDMConfiguration, 0, len(configurations))
	for _, configuration := range configurations {
		item, err := convertMDMConfiguration(configuration)
		if err != nil {
			return err
		}
		items = append(items, item)
	}
	return req.Write(types.MDMConfigurationList{Items: items})
}

func getMDMConfiguration(req api.Context, id uint) (*gtypes.MDMConfiguration, error) {
	configuration, err := req.GatewayClient.GetMDMConfiguration(req.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.NewErrNotFound("MDM configuration %d not found", id)
	}
	return configuration, err
}

// Get handles GET /api/mdm/configurations/{id}.
func (*MDMConfigurationsHandler) Get(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	configuration, err := getMDMConfiguration(req, id)
	if err != nil {
		return err
	}
	result, err := convertMDMConfiguration(*configuration)
	if err != nil {
		return err
	}
	return req.Write(result)
}

// Update handles PUT /api/mdm/configurations/{id}.
func (h *MDMConfigurationsHandler) Update(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	current, err := getMDMConfiguration(req, id)
	if err != nil {
		return err
	}
	var in types.MDMConfiguration
	if err := req.Read(&in); err != nil {
		return err
	}
	configuration, err := h.mdmConfigurationFromInput(req, in, current)
	if err != nil {
		return err
	}
	configuration.ID = id
	if err := req.GatewayClient.UpdateMDMConfiguration(req.Context(), configuration); errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("MDM configuration %d not found", id)
	} else if err != nil {
		return err
	}
	updated, err := getMDMConfiguration(req, id)
	if err != nil {
		return err
	}
	result, err := convertMDMConfiguration(*updated)
	if err != nil {
		return err
	}
	return req.Write(result)
}

// UpdateEnforcement handles PUT /api/mdm/configurations/{id}/enforcement. It
// updates only the enforcement policy (the enable toggle and the allowlist).
func (*MDMConfigurationsHandler) UpdateEnforcement(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	current, err := getMDMConfiguration(req, id)
	if err != nil {
		return err
	}
	var in types.MDMConfigurationEnforcementRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	allowlist, err := enforcementAllowlistForSave(in.EnforcementEnabled, in.EnforcementAllowlist, current)
	if err != nil {
		return err
	}
	if err := req.GatewayClient.UpdateMDMConfigurationEnforcement(req.Context(), id, in.EnforcementEnabled, allowlist); errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("MDM configuration %d not found", id)
	} else if err != nil {
		return err
	}
	updated, err := getMDMConfiguration(req, id)
	if err != nil {
		return err
	}
	result, err := convertMDMConfiguration(*updated)
	if err != nil {
		return err
	}
	return req.Write(result)
}

// Delete removes the configuration and its keys. Enrolled devices remain.
func (*MDMConfigurationsHandler) Delete(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	if err := req.GatewayClient.DeleteMDMConfiguration(req.Context(), id); err != nil {
		return err
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// DownloadArtifact serves a rendered ZIP by its platform/OS slug without
// reopening a source bundle or rendering templates.
func (*MDMConfigurationsHandler) DownloadArtifact(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	configuration, err := getMDMConfiguration(req, id)
	if err != nil {
		return err
	}
	slug := strings.TrimSpace(req.PathValue("slug"))
	index := slices.IndexFunc(configuration.Artifacts, func(artifact gtypes.MDMConfigurationArtifact) bool {
		return artifact.Slug == slug
	})
	if index < 0 {
		return types.NewErrNotFound("MDM configuration %d has no artifact %q", id, slug)
	}
	selected := configuration.Artifacts[index]

	sum := sha256.Sum256(selected.Content)
	if hex.EncodeToString(sum[:]) != selected.Digest {
		return fmt.Errorf("rendered MDM artifact %s failed its content digest check", selected.Slug)
	}
	filename := fmt.Sprintf("obot-sentry-%s.zip", selected.Slug)
	req.ResponseWriter.Header().Set("ETag", fmt.Sprintf("%q", selected.Digest))
	req.ResponseWriter.Header().Set("Content-Type", "application/zip")
	req.ResponseWriter.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeContent(req.ResponseWriter, req.Request, "", time.Time{}, bytes.NewReader(selected.Content))
	return nil
}

func (*MDMConfigurationsHandler) ListEnrollmentKeys(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	keys, err := req.GatewayClient.ListDeviceEnrollmentKeys(req.Context(), id)
	if err != nil {
		return err
	}
	items := make([]types.MDMEnrollmentKey, 0, len(keys))
	for _, key := range keys {
		items = append(items, convertMDMEnrollmentKey(key))
	}
	return req.Write(types.MDMEnrollmentKeyList{Items: items})
}

func (*MDMConfigurationsHandler) CreateEnrollmentKey(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	var in types.MDMEnrollmentKeyCreateRequest
	if err := req.Read(&in); err != nil {
		return err
	}
	var expiresAt *time.Time
	if in.ExpiresAt != nil {
		expiresAt = new(in.ExpiresAt.GetTime())
	}
	key, err := req.GatewayClient.CreateDeviceEnrollmentKey(req.Context(), id, req.UserID(), in.Name, expiresAt)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("MDM configuration %d not found", id)
	}
	if err != nil {
		return err
	}
	return req.WriteCreated(types.MDMEnrollmentKeyCreateResponse{
		MDMEnrollmentKey:     convertMDMEnrollmentKey(key.DeviceEnrollmentKey),
		EnrollmentCredential: key.EnrollmentCredential,
	})
}

func (*MDMConfigurationsHandler) DeleteEnrollmentKey(req api.Context) error {
	configurationID, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	keyID, err := parsePathUint(req, "key_id", "enrollment key id")
	if err != nil {
		return err
	}
	if err := req.GatewayClient.DeleteDeviceEnrollmentKey(req.Context(), configurationID, keyID); err != nil {
		return err
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (*MDMConfigurationsHandler) ListDevices(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	devices, err := req.GatewayClient.ListDevices(req.Context(), id)
	if err != nil {
		return err
	}
	items := make([]types.Device, 0, len(devices))
	for _, device := range devices {
		items = append(items, gtypes.ConvertDevice(device))
	}
	return req.Write(types.DeviceList{Items: items})
}

func convertMDMEnrollmentKey(key gtypes.DeviceEnrollmentKey) types.MDMEnrollmentKey {
	return types.MDMEnrollmentKey{
		ID:         key.ID,
		Name:       key.Name,
		CreatedAt:  *types.NewTime(key.CreatedAt),
		LastUsedAt: optionalAPITime(key.LastUsedAt),
		ExpiresAt:  optionalAPITime(key.ExpiresAt),
	}
}

func optionalAPITime(value *time.Time) *types.Time {
	if value == nil {
		return nil
	}
	return types.NewTime(*value)
}

func configurationIDFromPath(req api.Context) (uint, error) {
	return parsePathUint(req, "id", "MDM configuration id")
}

func parsePathUint(req api.Context, param, label string) (uint, error) {
	raw := req.PathValue(param)
	if raw == "" {
		return 0, types.NewErrBadRequest("missing %s", label)
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, types.NewErrBadRequest("invalid %s: %v", label, err)
	}
	return uint(id), nil
}

// mdmConfigurationFromInput validates administrator values against the latest
// source bundle and renders every platform/OS artifact before anything is
// persisted. Only creation may leave the configuration unconfigured.
func (h *MDMConfigurationsHandler) mdmConfigurationFromInput(req api.Context, in types.MDMConfiguration, current *gtypes.MDMConfiguration) (*gtypes.MDMConfiguration, error) {
	configuration := &gtypes.MDMConfiguration{
		AssetDigest: strings.TrimSpace(in.AssetDigest),
	}
	if configuration.AssetDigest == "" {
		if current != nil {
			return nil, types.NewErrBadRequest("assetDigest is required when updating an MDM configuration")
		}
		return configuration, nil
	}

	source, err := getMDMAssetSource(req)
	if err != nil {
		return nil, err
	}
	if source.Annotations[v1.MDMAssetSourceSyncAnnotation] == "true" {
		return nil, types.NewErrHTTP(http.StatusConflict, "the MDM asset source is refreshing; reload the configuration fields and try again")
	}
	if source.Status.LatestDigest != configuration.AssetDigest {
		return nil, types.NewErrHTTP(http.StatusConflict, "the selected MDM asset is no longer the latest; reload the configuration fields and try again")
	}

	bundle, err := req.GatewayClient.GetMDMAssetBundle(req.Context(), configuration.AssetDigest)
	if err != nil {
		return nil, err
	}
	loader, err := mdmassets.OpenArchive(bundle.Content)
	if err != nil {
		return nil, err
	}

	// Record the bundle's version alongside the rendered artifacts so the
	// generated version is reportable without reopening the bundle.
	configuration.ObotSentryVersion = loader.Manifest().ObotSentryVersion
	values, err := decodeMDMInputValues(in.Values)
	if err != nil {
		return nil, err
	}
	renderValues := make(map[string]any, len(values)+1)
	maps.Copy(renderValues, values)
	renderValues["serverURL"] = h.serverURL
	rendered, err := loader.RenderAll(renderValues)
	if err != nil {
		return nil, types.NewErrBadRequest("invalid configuration values: %v", err)
	}
	if len(rendered) == 0 {
		return nil, types.NewErrBadRequest("the latest MDM asset has no platform/OS configurations")
	}

	storedValues, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	configuration.Values = string(storedValues)
	configuration.Artifacts = make([]gtypes.MDMConfigurationArtifact, 0, len(rendered))
	for _, artifact := range rendered {
		configuration.Artifacts = append(configuration.Artifacts, gtypes.MDMConfigurationArtifact{
			Slug:         mdmassets.ArtifactSlug(artifact.Platform, artifact.OS),
			Platform:     artifact.Platform,
			OS:           artifact.OS,
			Instructions: artifact.Instructions,
			Content:      artifact.Content,
		})
	}
	return configuration, nil
}

func convertMDMConfiguration(configuration gtypes.MDMConfiguration) (types.MDMConfiguration, error) {
	result := types.MDMConfiguration{
		ID:                configuration.ID,
		IsDefault:         configuration.IsDefault,
		CreatedAt:         *types.NewTime(configuration.CreatedAt),
		ObotSentryVersion: configuration.ObotSentryVersion,
		MDMConfigurationManifest: types.MDMConfigurationManifest{
			AssetDigest: configuration.AssetDigest,
		},
		Artifacts:            make([]types.MDMConfigurationArtifact, 0, len(configuration.Artifacts)),
		EnforcementEnabled:   configuration.EnforcementEnabled,
		EnforcementAllowlist: configuration.EnforcementAllowlist,
	}
	if configuration.Values != "" {
		if !json.Valid([]byte(configuration.Values)) {
			return types.MDMConfiguration{}, fmt.Errorf("MDM configuration %d has invalid stored values", configuration.ID)
		}
		result.Values = json.RawMessage(configuration.Values)
	}
	for _, artifact := range configuration.Artifacts {
		result.Artifacts = append(result.Artifacts, types.MDMConfigurationArtifact{
			Slug:         artifact.Slug,
			Platform:     artifact.Platform,
			OS:           artifact.OS,
			Instructions: artifact.Instructions,
		})
	}
	return result, nil
}

func decodeMDMInputValues(raw json.RawMessage) (map[string]any, error) {
	values := make(map[string]any)
	if len(raw) != 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, types.NewErrBadRequest("values must be a JSON object: %v", err)
		}
	}
	delete(values, "serverURL")
	return values, nil
}

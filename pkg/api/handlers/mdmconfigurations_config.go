package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mdmassets"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"gorm.io/gorm"
)

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func (h *MDMConfigurationsHandler) mdmConfigurationForSave(req api.Context, in types.MDMConfiguration, current *gtypes.MDMConfiguration) (gtypes.MDMConfiguration, error) {
	configuration := gtypes.MDMConfiguration{
		Name:        strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description),
	}
	if configuration.Name == "" {
		return gtypes.MDMConfiguration{}, types.NewErrBadRequest("name is required")
	}

	in.AssetDigest = strings.TrimSpace(in.AssetDigest)
	in.Platform = strings.TrimSpace(in.Platform)
	in.OS = strings.TrimSpace(in.OS)
	if in.AssetDigest == "" && in.Platform == "" && in.OS == "" {
		return configuration, nil
	}
	if in.AssetDigest == "" || in.Platform == "" || in.OS == "" {
		return gtypes.MDMConfiguration{}, types.NewErrBadRequest("asset digest, platform, and os must all be set or all be blank")
	}
	targetChanged := current == nil ||
		current.AssetDigest != in.AssetDigest ||
		current.Platform != in.Platform ||
		current.OS != in.OS
	if targetChanged {
		source, err := getMDMAssetSource(req)
		if err != nil {
			return gtypes.MDMConfiguration{}, err
		}
		if source.Annotations[v1.MDMAssetSourceSyncAnnotation] == "true" {
			return gtypes.MDMConfiguration{}, types.NewErrHTTP(http.StatusConflict, "the MDM asset source is refreshing; reload the target list and try again")
		}
		if source.Status.LatestDigest != in.AssetDigest {
			return gtypes.MDMConfiguration{}, types.NewErrHTTP(http.StatusConflict, "the selected MDM asset is no longer the latest; reload the target list and try again")
		}
	}

	assetConfiguration, storedValues, err := h.validateMDMConfiguration(req, in)
	if err != nil {
		return gtypes.MDMConfiguration{}, err
	}
	configuration.AssetDigest = in.AssetDigest
	configuration.Platform = assetConfiguration.Platform
	configuration.OS = assetConfiguration.OS
	configuration.Values = storedValues
	return configuration, nil
}

func (h *MDMConfigurationsHandler) mdmConfigurationFromGateway(req api.Context, configuration gtypes.MDMConfiguration, render bool) types.MDMConfiguration {
	result := types.MDMConfiguration{
		ID:          configuration.ID,
		Name:        configuration.Name,
		Description: configuration.Description,
		CreatedAt:   *types.NewTime(configuration.CreatedAt),
		AssetDigest: configuration.AssetDigest,
		Platform:    configuration.Platform,
		OS:          configuration.OS,
	}
	if configuration.Values != "" {
		if !json.Valid([]byte(configuration.Values)) {
			result.Error = "The saved MDM configuration values are invalid."
			return result
		}
		result.Values = json.RawMessage(configuration.Values)
	}

	hasDigest := configuration.AssetDigest != ""
	hasPlatform := configuration.Platform != ""
	hasOS := configuration.OS != ""
	if !hasDigest && !hasPlatform && !hasOS || !render {
		return result
	}
	if hasDigest != hasPlatform || hasDigest != hasOS {
		result.Error = "The saved MDM asset selection is incomplete."
		return result
	}

	loader, err := h.openMDMAssets(req, configuration.AssetDigest)
	if err != nil {
		result.Error = "The saved MDM asset is unavailable."
		return result
	}
	assetConfiguration, err := loader.Find(configuration.Platform, configuration.OS)
	if err != nil {
		result.Error = "The saved platform and OS are unavailable in the pinned MDM asset."
		return result
	}
	storedValues, err := decodeStoredMDMValues(configuration.Values)
	if err != nil {
		result.Error = "The saved MDM configuration values are invalid."
		return result
	}
	_, instructions, err := h.completeAndRender(loader, assetConfiguration, storedValues)
	if err != nil {
		result.Error = "The saved MDM configuration could not be rendered."
		return result
	}
	result.Instructions = instructions
	return result
}

// DownloadConfig builds a ZIP exclusively from the configuration's pinned
// bundle and saved target.
func (h *MDMConfigurationsHandler) DownloadConfig(req api.Context) error {
	id, err := configurationIDFromPath(req)
	if err != nil {
		return err
	}
	configuration, err := h.getConfiguration(req, id)
	if err != nil {
		return err
	}
	if configuration.Platform == "" || configuration.OS == "" || configuration.AssetDigest == "" {
		return types.NewErrHTTP(http.StatusConflict, "this MDM configuration has no saved MDM asset")
	}
	loader, err := h.openMDMAssets(req, configuration.AssetDigest)
	if err != nil {
		return types.NewErrHTTP(http.StatusConflict, "the saved MDM asset bundle is unavailable")
	}
	assetConfiguration, err := loader.Find(configuration.Platform, configuration.OS)
	if err != nil {
		return types.NewErrHTTP(http.StatusConflict, "the saved platform and OS are unavailable in their pinned MDM assets")
	}
	storedValues, err := decodeStoredMDMValues(configuration.Values)
	if err != nil {
		return fmt.Errorf("decoding saved MDM configuration values: %w", err)
	}
	values, _, err := h.completeAndRender(loader, assetConfiguration, storedValues)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("obot-sentry-%s-%s-%s.zip", assetConfiguration.Platform, assetConfiguration.OS, slugify(configuration.Name))
	req.ResponseWriter.Header().Set("Content-Type", "application/zip")
	req.ResponseWriter.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	req.WriteHeader(http.StatusOK)
	return loader.Zip(req.ResponseWriter, assetConfiguration, values)
}

func (h *MDMConfigurationsHandler) validateMDMConfiguration(req api.Context, in types.MDMConfiguration) (types.MDMAssetConfiguration, string, error) {
	digest := strings.TrimSpace(in.AssetDigest)
	if digest == "" {
		return types.MDMAssetConfiguration{}, "", types.NewErrBadRequest("asset digest is required")
	}
	loader, err := h.openMDMAssets(req, digest)
	if err != nil {
		return types.MDMAssetConfiguration{}, "", types.NewErrHTTP(http.StatusConflict, "the selected MDM asset is unavailable")
	}
	assetConfiguration, err := loader.Find(strings.TrimSpace(in.Platform), strings.TrimSpace(in.OS))
	if err != nil {
		return types.MDMAssetConfiguration{}, "", types.NewErrBadRequest("%v", err)
	}
	values, err := decodeMDMInputValues(in.Values)
	if err != nil {
		return types.MDMAssetConfiguration{}, "", err
	}
	if _, _, err := h.completeAndRender(loader, assetConfiguration, values); err != nil {
		return types.MDMAssetConfiguration{}, "", err
	}
	storedValues, err := json.Marshal(values)
	if err != nil {
		return types.MDMAssetConfiguration{}, "", err
	}
	return assetConfiguration, string(storedValues), nil
}

func (h *MDMConfigurationsHandler) completeAndRender(loader *mdmassets.Loader, assetConfiguration types.MDMAssetConfiguration, adminValues map[string]any) (map[string]any, string, error) {
	values := make(map[string]any, len(adminValues)+1)
	for name, value := range adminValues {
		values[name] = value
	}
	values["serverURL"] = h.serverURL
	if err := loader.CompleteValues(values); err != nil {
		return nil, "", types.NewErrBadRequest("invalid configuration values: %v", err)
	}
	instructions, err := loader.RenderInstructions(assetConfiguration, values)
	if err != nil {
		return nil, "", err
	}
	if err := loader.ValidateTemplates(assetConfiguration, values); err != nil {
		return nil, "", err
	}
	return values, instructions, nil
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

func decodeStoredMDMValues(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	values := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = map[string]any{}
	}
	return values, nil
}

func (h *MDMConfigurationsHandler) getConfiguration(req api.Context, id uint) (*gtypes.MDMConfiguration, error) {
	configuration, err := req.GatewayClient.GetMDMConfiguration(req.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.NewErrNotFound("MDM configuration %d not found", id)
	}
	return configuration, err
}

func (*MDMConfigurationsHandler) openMDMAssets(req api.Context, digest string) (*mdmassets.Loader, error) {
	bundle, err := req.GatewayClient.GetMDMAssetBundle(req.Context(), digest)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bundle.Content)
	if hex.EncodeToString(sum[:]) != bundle.Digest {
		return nil, fmt.Errorf("MDM asset bundle %s failed its content digest check", shortMDMDigest(bundle.Digest))
	}
	return mdmassets.OpenArchive(bundle.Content)
}

func shortMDMDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

func slugify(name string) string {
	slug := slugNonAlnum.ReplaceAllString(strings.ToLower(name), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "configuration"
	}
	return slug
}

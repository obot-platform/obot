package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mdmassets"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const handlerTestStringFields = `{
  "type":"object",
  "additionalProperties":false,
  "required":["serverURL"],
  "properties":{
    "serverURL":{"type":"string","format":"uri"},
    "message":{"type":"string","default":"default"}
  }
}`

func TestMDMConfigurationCreateGetAndDownloadUsePinnedAsset(t *testing.T) {
	env := newHandlerTestEnvironment(t)
	pinnedDigest := env.addAsset(t, handlerTestBundle(t, "pinned", "intune", "windows", "pinned-package"))
	env.setLatest(t, pinnedDigest)

	createRecorder := httptest.NewRecorder()
	createRequest := env.request(t, http.MethodPost, "/api/mdm/configurations", apitypes.MDMConfiguration{
		Name:        "  Configured fleet  ",
		Description: "  Managed Windows devices  ",
		AssetDigest: pinnedDigest,
		Platform:    " intune ",
		OS:          " windows ",
		Values:      json.RawMessage(`{"message":"initial","serverURL":"https://caller.invalid"}`),
	})
	require.NoError(t, env.handler.Create(env.context(createRequest, createRecorder)))
	assert.Equal(t, http.StatusCreated, createRecorder.Code)

	var created apitypes.MDMConfigurationCreateResponse
	require.NoError(t, json.Unmarshal(createRecorder.Body.Bytes(), &created))
	assert.Equal(t, "Configured fleet", created.Name)
	assert.Equal(t, "Managed Windows devices", created.Description)
	assert.Equal(t, pinnedDigest, created.AssetDigest)
	assert.Equal(t, "intune", created.Platform)
	assert.Equal(t, "windows", created.OS)
	assert.Empty(t, created.Error)
	assert.Contains(t, created.Instructions, "bundle=pinned")
	assert.Contains(t, created.Instructions, "server=https://obot.example")
	assert.True(t, strings.HasPrefix(created.EnrollmentCredential, "ode1-"))
	assertSavedHandlerTestValues(t, created.Values, "initial")

	stored := env.getConfiguration(t, created.ID)
	assert.Equal(t, pinnedDigest, stored.AssetDigest)
	assert.Equal(t, "intune", stored.Platform)
	assert.Equal(t, "windows", stored.OS)
	assert.JSONEq(t, `{"message":"initial"}`, stored.Values)

	newDigest := env.addAsset(t, handlerTestBundle(t, "latest", "intune", "windows", "latest-package"))
	require.NotEqual(t, pinnedDigest, newDigest)
	env.setLatest(t, newDigest)

	getRecorder := httptest.NewRecorder()
	require.NoError(t, env.handler.Get(env.configurationContext(http.MethodGet, created.ID, "", nil, getRecorder)))
	assert.Equal(t, http.StatusOK, getRecorder.Code)
	got := decodeHandlerTestConfiguration(t, getRecorder)
	assert.Equal(t, pinnedDigest, got.AssetDigest)
	assert.Contains(t, got.Instructions, "bundle=pinned")
	assert.NotContains(t, got.Instructions, "bundle=latest")
	assert.Empty(t, got.Error)
	assertSavedHandlerTestValues(t, got.Values, "initial")

	downloadRecorder := httptest.NewRecorder()
	require.NoError(t, env.handler.DownloadConfig(env.configurationContext(http.MethodGet, created.ID, "/download", nil, downloadRecorder)))
	assert.Equal(t, http.StatusOK, downloadRecorder.Code)
	assert.Equal(t, "application/zip", downloadRecorder.Header().Get("Content-Type"))
	assert.Empty(t, downloadRecorder.Header().Get("Content-Length"))
	assert.Contains(t, downloadRecorder.Header().Get("Content-Disposition"), "obot-sentry-intune-windows-configured-fleet.zip")
	assert.Equal(t, "pinned-package", downloadedHandlerTestFile(t, downloadRecorder.Body.Bytes(), "package.bin"))
}

func TestMDMConfigurationUpdateSetsAndClearsAssetSelection(t *testing.T) {
	env := newHandlerTestEnvironment(t)
	configuration := env.createBlankConfiguration(t, "Original fleet")
	digest := env.addAsset(t, handlerTestBundle(t, "updated", "jamf", "macos", "mac-package"))
	env.setLatest(t, digest)

	updateRecorder := env.updateConfiguration(t, configuration.ID, apitypes.MDMConfiguration{
		Name:        "Updated fleet",
		Description: "Mac devices",
		AssetDigest: digest,
		Platform:    "jamf",
		OS:          "macos",
		Values:      json.RawMessage(`{"message":"updated"}`),
	})
	assert.Equal(t, http.StatusOK, updateRecorder.Code)
	updated := decodeHandlerTestConfiguration(t, updateRecorder)
	assert.Equal(t, configuration.ID, updated.ID)
	assert.Equal(t, "Updated fleet", updated.Name)
	assert.Equal(t, digest, updated.AssetDigest)
	assert.Equal(t, "jamf", updated.Platform)
	assert.Equal(t, "macos", updated.OS)
	assert.Contains(t, updated.Instructions, "bundle=updated")
	assert.Empty(t, updated.Error)
	assertSavedHandlerTestValues(t, updated.Values, "updated")

	stored := env.getConfiguration(t, configuration.ID)
	assert.Equal(t, digest, stored.AssetDigest)
	assert.JSONEq(t, `{"message":"updated"}`, stored.Values)

	clearRecorder := env.updateConfiguration(t, configuration.ID, apitypes.MDMConfiguration{
		Name:        "Updated fleet",
		Description: "No asset selected",
	})
	cleared := decodeHandlerTestConfiguration(t, clearRecorder)
	assert.Equal(t, "No asset selected", cleared.Description)
	assert.Empty(t, cleared.AssetDigest)
	assert.Empty(t, cleared.Platform)
	assert.Empty(t, cleared.OS)
	assert.Empty(t, cleared.Values)
	assert.Empty(t, cleared.Instructions)
	assert.Empty(t, cleared.Error)

	stored = env.getConfiguration(t, configuration.ID)
	assert.Empty(t, stored.AssetDigest)
	assert.Empty(t, stored.Platform)
	assert.Empty(t, stored.OS)
	assert.Empty(t, stored.Values)

	downloadRecorder := httptest.NewRecorder()
	err := env.handler.DownloadConfig(env.configurationContext(http.MethodGet, configuration.ID, "/download", nil, downloadRecorder))
	requireHandlerTestHTTPError(t, err, http.StatusConflict)
}

func TestMDMConfigurationUpdateCanPreserveOlderPinnedAsset(t *testing.T) {
	env := newHandlerTestEnvironment(t)
	digest := env.addAsset(t, handlerTestBundle(t, "kept", "intune", "windows", "kept-package"))
	env.setLatest(t, digest)
	configuration := env.createConfiguration(t, apitypes.MDMConfiguration{
		Name:        "Keep me",
		Description: "Original description",
		AssetDigest: digest,
		Platform:    "intune",
		OS:          "windows",
		Values:      json.RawMessage(`{"message":"kept"}`),
	})
	latestDigest := env.addAsset(t, handlerTestBundle(t, "latest", "intune", "windows", "latest-package"))
	env.setLatest(t, latestDigest)

	recorder := env.updateConfiguration(t, configuration.ID, apitypes.MDMConfiguration{
		Name:        "Renamed",
		Description: "Updated description",
		AssetDigest: digest,
		Platform:    "intune",
		OS:          "windows",
		Values:      json.RawMessage(`{"message":"replacement"}`),
	})
	assert.Equal(t, http.StatusOK, recorder.Code)
	updated := decodeHandlerTestConfiguration(t, recorder)
	assert.Equal(t, "Renamed", updated.Name)
	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, digest, updated.AssetDigest)
	assert.Contains(t, updated.Instructions, "bundle=kept")
	assertSavedHandlerTestValues(t, updated.Values, "replacement")

	stored := env.getConfiguration(t, configuration.ID)
	assert.Equal(t, "Renamed", stored.Name)
	assert.Equal(t, "Updated description", stored.Description)
	assert.Equal(t, digest, stored.AssetDigest)
	assert.Equal(t, "intune", stored.Platform)
	assert.Equal(t, "windows", stored.OS)
	assert.JSONEq(t, `{"message":"replacement"}`, stored.Values)

	getRecorder := httptest.NewRecorder()
	require.NoError(t, env.handler.Get(env.configurationContext(http.MethodGet, configuration.ID, "", nil, getRecorder)))
	got := decodeHandlerTestConfiguration(t, getRecorder)
	assert.Contains(t, got.Instructions, "bundle=kept")
	assert.Empty(t, got.Error)
}

func TestMDMConfigurationInvalidTargetChangePreservesExistingConfiguration(t *testing.T) {
	env := newHandlerTestEnvironment(t)
	pinnedDigest := env.addAsset(t, handlerTestBundle(t, "kept", "intune", "windows", "kept-package"))
	env.setLatest(t, pinnedDigest)
	configuration := env.createConfiguration(t, apitypes.MDMConfiguration{
		Name:        "Keep me",
		Description: "Original description",
		AssetDigest: pinnedDigest,
		Platform:    "intune",
		OS:          "windows",
		Values:      json.RawMessage(`{"message":"kept"}`),
	})
	staleDigest := env.addAsset(t, handlerTestBundle(t, "stale", "intune", "windows", "stale-package"))
	latestDigest := env.addAsset(t, handlerTestBundle(t, "latest", "intune", "windows", "latest-package"))
	env.setLatest(t, latestDigest)

	request := env.request(t, http.MethodPut, "/api/mdm/configurations/"+strconv.FormatUint(uint64(configuration.ID), 10), apitypes.MDMConfiguration{
		Name:        "Do not save",
		AssetDigest: staleDigest,
		Platform:    "intune",
		OS:          "windows",
		Values:      json.RawMessage(`{"message":"replacement"}`),
	})
	request.SetPathValue("id", strconv.FormatUint(uint64(configuration.ID), 10))
	err := env.handler.Update(env.context(request, httptest.NewRecorder()))
	requireHandlerTestHTTPError(t, err, http.StatusConflict)

	stored := env.getConfiguration(t, configuration.ID)
	assert.Equal(t, "Keep me", stored.Name)
	assert.Equal(t, "Original description", stored.Description)
	assert.Equal(t, pinnedDigest, stored.AssetDigest)
	assert.Equal(t, "intune", stored.Platform)
	assert.Equal(t, "windows", stored.OS)
	assert.JSONEq(t, `{"message":"kept"}`, stored.Values)
}

func TestMDMConfigurationGetSurfacesUnavailablePinnedAsset(t *testing.T) {
	env := newHandlerTestEnvironment(t)
	configuration, _, err := env.gateway.CreateMDMConfiguration(t.Context(), 1, gatewaytypes.MDMConfiguration{
		Name:        "Broken pin",
		AssetDigest: "missing-digest",
		Platform:    "intune",
		OS:          "windows",
		Values:      `{"message":"kept"}`,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	require.NoError(t, env.handler.Get(env.configurationContext(http.MethodGet, configuration.ID, "", nil, recorder)))
	assert.Equal(t, http.StatusOK, recorder.Code)
	got := decodeHandlerTestConfiguration(t, recorder)
	assert.Equal(t, "missing-digest", got.AssetDigest)
	assert.Empty(t, got.Instructions)
	assert.Equal(t, "The saved MDM asset is unavailable.", got.Error)
	assert.JSONEq(t, `{"message":"kept"}`, string(got.Values))

	downloadRecorder := httptest.NewRecorder()
	err = env.handler.DownloadConfig(env.configurationContext(http.MethodGet, configuration.ID, "/download", nil, downloadRecorder))
	requireHandlerTestHTTPError(t, err, http.StatusConflict)
}

type handlerTestEnvironment struct {
	gateway *gatewayclient.Client
	storage storage.Client
	handler *MDMConfigurationsHandler
}

func newHandlerTestEnvironment(t *testing.T) *handlerTestEnvironment {
	t.Helper()
	storageClient := storage.Client(newFakeStorage(t, &v1.MDMAssetSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultMDMAssetSource,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MDMAssetSourceSpec{
			MDMAssetSourceManifest: apitypes.MDMAssetSourceManifest{
				Source: "https://example.test/mdm-assets.tar.gz",
			},
		},
	}))
	return &handlerTestEnvironment{
		gateway: newHandlerTestGateway(t),
		storage: storageClient,
		handler: NewMDMConfigurationsHandler("https://obot.example"),
	}
}

func (e *handlerTestEnvironment) addAsset(t *testing.T, content []byte) string {
	t.Helper()
	digest, err := e.gateway.StoreMDMAssetBundle(t.Context(), content)
	require.NoError(t, err)
	loader, err := mdmassets.OpenArchive(content)
	require.NoError(t, err)
	require.NoError(t, e.storage.Create(t.Context(), &v1.MDMAsset{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v1.MDMAssetName(digest),
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MDMAssetSpec{
			Digest:           digest,
			MDMAssetManifest: loader.Manifest(),
		},
	}))
	return digest
}

func (e *handlerTestEnvironment) setLatest(t *testing.T, digest string) {
	t.Helper()
	var source v1.MDMAssetSource
	require.NoError(t, e.storage.Get(t.Context(), kclient.ObjectKey{
		Name:      system.DefaultMDMAssetSource,
		Namespace: system.DefaultNamespace,
	}, &source))
	source.Status.LatestDigest = digest
	source.Status.LastSyncTime = metav1.Now()
	require.NoError(t, e.storage.Update(t.Context(), &source))
}

func (e *handlerTestEnvironment) createBlankConfiguration(t *testing.T, name string) *gatewaytypes.MDMConfiguration {
	t.Helper()
	configuration, _, err := e.gateway.CreateMDMConfiguration(t.Context(), 1, gatewaytypes.MDMConfiguration{Name: name})
	require.NoError(t, err)
	return configuration
}

func (e *handlerTestEnvironment) createConfiguration(t *testing.T, input apitypes.MDMConfiguration) apitypes.MDMConfiguration {
	t.Helper()
	recorder := httptest.NewRecorder()
	require.NoError(t, e.handler.Create(e.context(e.request(t, http.MethodPost, "/api/mdm/configurations", input), recorder)))
	var response apitypes.MDMConfigurationCreateResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response.MDMConfiguration
}

func (e *handlerTestEnvironment) updateConfiguration(t *testing.T, id uint, input apitypes.MDMConfiguration) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := e.request(t, http.MethodPut, "/api/mdm/configurations/"+strconv.FormatUint(uint64(id), 10), input)
	request.SetPathValue("id", strconv.FormatUint(uint64(id), 10))
	require.NoError(t, e.handler.Update(e.context(request, recorder)))
	return recorder
}

func (e *handlerTestEnvironment) getConfiguration(t *testing.T, id uint) *gatewaytypes.MDMConfiguration {
	t.Helper()
	configuration, err := e.gateway.GetMDMConfiguration(t.Context(), id)
	require.NoError(t, err)
	return configuration
}

func (e *handlerTestEnvironment) request(t *testing.T, method, path string, value any) *http.Request {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return httptest.NewRequest(method, path, bytes.NewReader(body))
}

func (e *handlerTestEnvironment) context(request *http.Request, recorder *httptest.ResponseRecorder) api.Context {
	return api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        e.storage,
		GatewayClient:  e.gateway,
		User:           &user.DefaultInfo{UID: "42"},
	}
}

func (e *handlerTestEnvironment) configurationContext(method string, id uint, suffix string, body io.Reader, recorder *httptest.ResponseRecorder) api.Context {
	if body == nil {
		body = http.NoBody
	}
	request := httptest.NewRequest(method, "/api/mdm/configurations/"+strconv.FormatUint(uint64(id), 10)+suffix, body)
	request.SetPathValue("id", strconv.FormatUint(uint64(id), 10))
	return e.context(request, recorder)
}

func newHandlerTestGateway(t *testing.T) *gatewayclient.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, false)
	t.Cleanup(func() {
		_ = gateway.Close()
	})
	return gateway
}

func handlerTestBundle(t *testing.T, marker, platform, osName, packageContent string) []byte {
	t.Helper()
	return handlerTestBundleWithFields(t, marker, platform, osName, packageContent, handlerTestStringFields)
}

func handlerTestBundleWithFields(t *testing.T, marker, platform, osName, packageContent, fields string) []byte {
	t.Helper()
	dir := t.TempDir()
	writeHandlerTestFile(t, filepath.Join(dir, "package.bin"), packageContent)
	writeHandlerTestFile(t, filepath.Join(dir, "INSTRUCTIONS.md.tmpl"), fmt.Sprintf("bundle=%s server={{.serverURL}} message={{.message}}", marker))
	manifest := fmt.Sprintf(`{
  "schemaVersion":"v1",
  "obotSentryVersion":"test",
  "fields":%s,
  "platforms":[{"id":%q,"label":%q}],
  "configurations":[{"platform":%q,"os":%q,"osLabel":%q,"instructions":"INSTRUCTIONS.md.tmpl","assets":["package.bin","INSTRUCTIONS.md.tmpl"]}]
}`, fields, platform, platform, platform, osName, osName)
	writeHandlerTestFile(t, filepath.Join(dir, "manifest.json"), manifest)
	content, err := mdmassets.Import(t.Context(), dir)
	require.NoError(t, err)
	return content
}

func assertSavedHandlerTestValues(t *testing.T, raw json.RawMessage, message string) {
	t.Helper()
	var values map[string]any
	require.NoError(t, json.Unmarshal(raw, &values))
	assert.Equal(t, message, values["message"])
	assert.NotContains(t, values, "serverURL")
}

func decodeHandlerTestConfiguration(t *testing.T, recorder *httptest.ResponseRecorder) apitypes.MDMConfiguration {
	t.Helper()
	var configuration apitypes.MDMConfiguration
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &configuration))
	return configuration
}

func requireHandlerTestHTTPError(t *testing.T, err error, code int) {
	t.Helper()
	var httpErr *apitypes.ErrHTTP
	require.True(t, errors.As(err, &httpErr), "error = %v, want HTTP %d", err, code)
	assert.Equal(t, code, httpErr.Code)
}

func writeHandlerTestFile(t *testing.T, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
	require.NoError(t, os.WriteFile(name, []byte(content), 0o644))
}

func downloadedHandlerTestFile(t *testing.T, archive []byte, name string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		opened, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(opened)
		closeErr := opened.Close()
		require.NoError(t, err)
		require.NoError(t, closeErr)
		return string(content)
	}
	t.Fatalf("download does not contain %q", name)
	return ""
}

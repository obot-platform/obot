package registry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Only Get and List are implemented. Any write, credential access (the gateway
// client is nil), or runtime action during discovery fails the endpoint tests.
type registryTestStorage struct {
	kclient.WithWatch
	reader  kclient.Reader
	getErr  error
	listErr error
}

func (s *registryTestStorage) Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	if s.getErr != nil {
		return s.getErr
	}
	return s.reader.Get(ctx, key, obj, opts...)
}

func (s *registryTestStorage) List(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) error {
	if s.listErr != nil {
		return s.listErr
	}
	return s.reader.List(ctx, list, opts...)
}

func TestRegistryVMCPVisibility(t *testing.T) {
	definitions := []*v1.VMCP{
		registryDefinition("vmcp1public", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"}),
		registryDefinition("vmcp1personal", "user-1", types.Subject{Type: types.SubjectTypeSelector, ID: "*"}),
		registryDefinition("vmcp1otherpersonal", "user-2", types.Subject{Type: types.SubjectTypeSelector, ID: "*"}),
		registryDefinition("vmcp1user", "", types.Subject{Type: types.SubjectTypeUser, ID: "user-1"}),
		registryDefinition("vmcp1obotgroup", "", types.Subject{Type: types.SubjectTypeObotGroup, ID: "engineering"}),
		registryDefinition("vmcp1idpgroup", "", types.Subject{Type: types.SubjectTypeGroup, ID: "developers"}),
		registryDefinition("vmcp1admin", "", types.Subject{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}),
		registryDefinition("vmcp1closed", ""),
		registryDefinition("vmcp1anonymoususer", "", types.Subject{Type: types.SubjectTypeUser, ID: "anonymous"}),
		registryDefinition("vmcp1anonymousgroup", "", types.Subject{Type: types.SubjectTypeGroup, ID: "everyone"}),
		registryDefinition("vmcp1userstar", "", types.Subject{Type: types.SubjectTypeUser, ID: "*"}),
	}
	objects := make([]kclient.Object, 0, len(definitions))
	for _, definition := range definitions {
		objects = append(objects, definition)
	}
	storage := newRegistryTestStorage(objects...)
	for _, tc := range []struct {
		name string
		user user.Info
		want []string
	}{
		{
			name: "nil anonymous user",
			want: []string{"vmcp1public"},
		},
		{
			name: "anonymous identity and groups cannot grant private access",
			user: &user.DefaultInfo{
				UID:    "anonymous",
				Groups: []string{authz.UnauthenticatedGroup},
				Extra:  map[string][]string{"auth_provider_groups": {"everyone"}},
			},
			want: []string{"vmcp1public"},
		},
		{
			name: "identity without authenticated group cannot grant private access",
			user: &user.DefaultInfo{
				UID: "user-1",
			},
			want: []string{"vmcp1public"},
		},
		{
			name: "authenticated personal and user profiles",
			user: &user.DefaultInfo{
				UID:    "user-1",
				Groups: []string{types.GroupAuthenticated},
			},
			want: []string{"vmcp1personal", "vmcp1public", "vmcp1user"},
		},
		{
			name: "Obot and identity provider groups",
			user: &user.DefaultInfo{
				UID:    "user-3",
				Groups: []string{types.GroupAuthenticated},
				Extra: map[string][]string{
					"obot_groups":          {"engineering"},
					"auth_provider_groups": {"developers"},
				},
			},
			want: []string{"vmcp1idpgroup", "vmcp1obotgroup", "vmcp1public"},
		},
		{
			name: "revoked group membership",
			user: &user.DefaultInfo{
				UID:    "user-3",
				Groups: []string{types.GroupAuthenticated},
			},
			want: []string{"vmcp1public"},
		},
		{
			name: "admin must match consumption profiles",
			user: &user.DefaultInfo{
				UID:    "admin",
				Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
				Extra:  map[string][]string{"obot_groups": {types.GroupAdmin}},
			},
			want: []string{"vmcp1admin", "vmcp1public"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := registryRequest(t, storage, tc.user, "/v0.1/servers")
			var list types.RegistryServerList
			decodeRegistryResponse(t, response, &list)
			got := registryIDs(list)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("listed IDs = %v, want %v", got, tc.want)
			}
			for _, definition := range definitions {
				for _, suffix := range []string{"/versions", "/versions/latest"} {
					response := registryRequest(t, storage, tc.user, registryDetailPath(definition.Name)+suffix)
					if !slices.Contains(tc.want, definition.Name) {
						if response.Code != http.StatusNotFound {
							t.Fatalf("%s: status = %d, want 404", definition.Name+suffix, response.Code)
						}
						continue
					}
					if suffix == "/versions" {
						var versions types.RegistryServerList
						decodeRegistryResponse(t, response, &versions)
						if !reflect.DeepEqual(versions.Servers, []types.RegistryServerResponse{listedRegistryServer(t, list, definition.Name)}) {
							t.Fatalf("versions differ from listing: %#v", versions)
						}
					} else {
						var detail types.RegistryServerResponse
						decodeRegistryResponse(t, response, &detail)
						if !reflect.DeepEqual(detail, listedRegistryServer(t, list, definition.Name)) {
							t.Fatalf("detail differs from listing: %#v", detail)
						}
						assertRegistryRemote(t, detail, definition.Name)
					}
				}
			}
		})
	}
}

func TestRegistryOnlyPublishesDefinitions(t *testing.T) {
	definition := registryDefinition("vmcp1public", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	definition.Spec.LegacySlug = "ms1migrated"
	personal := registryDefinition("vmcp1private", "user-1")
	personal.Spec.LegacySlug = "ms1privatealias"
	storage := newRegistryTestStorage(
		definition,
		personal,
		&v1.VMCPInstance{
			ObjectMeta: registryObjectMeta("vmcpi1connection"),
			Spec: v1.VMCPInstanceSpec{
				UserID:     "user-1",
				LegacySlug: "ms1connectionalias",
				Manifest:   types.VMCPInstanceManifest{VMCPID: definition.Name},
			},
		},
		&v1.VMCPInstance{
			ObjectMeta: registryObjectMeta("vmcpi1secondconnection"),
			Spec: v1.VMCPInstanceSpec{
				UserID:   "user-2",
				Manifest: types.VMCPInstanceManifest{VMCPID: definition.Name},
			},
		},
		&v1.MCPServerCatalogEntry{ObjectMeta: registryObjectMeta("entry-template")},
		&v1.MCPServer{ObjectMeta: registryObjectMeta("ms1legacy")},
		&v1.MCPServer{
			ObjectMeta: registryObjectMeta("ms1component"),
			Spec: v1.MCPServerSpec{
				VMCPID:          definition.Name,
				VMCPComponentID: "component-1",
			},
		},
	)
	for _, u := range []user.Info{nil, &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupAuthenticated}}} {
		response := registryRequest(t, storage, u, "/v0.1/servers")
		var list types.RegistryServerList
		decodeRegistryResponse(t, response, &list)
		want := []string{"vmcp1public"}
		if u != nil {
			want = []string{"vmcp1private", "vmcp1public"}
		}
		if !slices.Equal(registryIDs(list), want) {
			t.Fatalf("listed IDs = %v, want %v", registryIDs(list), want)
		}
		for _, suffix := range []string{"/versions", "/versions/latest"} {
			response = registryRequest(t, storage, u, registryDetailPath("ms1migrated")+suffix)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "com.example.obot/vmcp1public") || strings.Contains(response.Body.String(), "ms1migrated") {
				t.Fatalf("alias must return canonical definition: %d %s", response.Code, response.Body.String())
			}
			for _, id := range []string{"vmcpi1connection", "vmcpi1secondconnection", "ms1connectionalias", "entry-template", "ms1legacy", "ms1component", "vmcp1missing"} {
				response = registryRequest(t, storage, u, registryDetailPath(id)+suffix)
				if response.Code != http.StatusNotFound {
					t.Fatalf("%s: status = %d, want 404", id+suffix, response.Code)
				}
			}
		}
	}
	for _, name := range []string{"ms1privatealias", "vmcp1private"} {
		response := registryRequest(t, storage, nil, registryDetailPath(name)+"/versions/latest")
		if response.Code != http.StatusNotFound {
			t.Fatalf("anonymous lookup of %s: status = %d, want 404", name, response.Code)
		}
	}
	for _, path := range []string{
		registryDetailPath(definition.Name) + "/versions/1.0.0",
		"/v0.1/servers/invalid/versions/latest",
		"/v0.1/servers/" + url.PathEscape("com.other/"+definition.Name) + "/versions/latest",
	} {
		response := registryRequest(t, storage, nil, path)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", path, response.Code)
		}
	}
}

func TestRegistrySearchAndPagination(t *testing.T) {
	alpha := registryDefinition("vmcp1alpha", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	alpha.Spec.Manifest.DisplayName = "Weather Tools"
	beta := registryDefinition("vmcp1beta", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	beta.Spec.Manifest.Description = "Forecasts and reports"
	gamma := registryDefinition("vmcp1gamma", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	// All three have equal timestamps; pagination must use the resource name as a tie breaker.
	storage := newRegistryTestStorage(gamma, beta, alpha)
	var collected []string
	cursor := ""
	for range 3 {
		response := registryRequest(t, storage, nil, "/v0.1/servers?limit=1&cursor="+url.QueryEscape(cursor))
		var list types.RegistryServerList
		decodeRegistryResponse(t, response, &list)
		if list.Metadata == nil || list.Metadata.Count != 1 {
			t.Fatalf("unexpected page metadata: %#v", list)
		}
		collected = append(collected, registryIDs(list)...)
		cursor = list.Metadata.NextCursor
	}
	if !slices.Equal(collected, []string{"vmcp1alpha", "vmcp1beta", "vmcp1gamma"}) || cursor != "" {
		t.Fatalf("paginated IDs = %v, final cursor = %q", collected, cursor)
	}
	for _, tc := range []struct {
		search string
		want   []string
	}{
		{
			search: "ALPHA",
			want:   []string{"vmcp1alpha"},
		},
		{
			search: "WEATHER",
			want:   []string{"vmcp1alpha"},
		},
		{
			search: "FORECAST",
			want:   []string{"vmcp1beta"},
		},
		{
			search: "nonexistent",
			want:   []string{},
		},
	} {
		response := registryRequest(t, storage, nil, "/v0.1/servers?search="+url.QueryEscape(tc.search))
		var list types.RegistryServerList
		decodeRegistryResponse(t, response, &list)
		if !slices.Equal(registryIDs(list), tc.want) {
			t.Fatalf("search %q: IDs = %v, want %v", tc.search, registryIDs(list), tc.want)
		}
		if len(tc.want) == 0 && (list.Servers == nil || !strings.Contains(response.Body.String(), `"servers":[]`)) {
			t.Fatalf("empty search result must be an array: %s", response.Body.String())
		}
	}
	response := registryRequest(t, storage, nil, "/v0.1/servers?cursor="+url.QueryEscape("com.example.obot/vmcp1gamma"))
	var last types.RegistryServerList
	decodeRegistryResponse(t, response, &last)
	if last.Servers == nil || len(last.Servers) != 0 || last.Metadata.NextCursor != "" {
		t.Fatalf("unexpected final page: %#v", last)
	}
}

func TestRegistryPaginationChronologicalOrder(t *testing.T) {
	alpha := registryDefinition("vmcp1alpha", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	beta := registryDefinition("vmcp1beta", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	alpha.CreationTimestamp = metav1.NewTime(time.Unix(200, 0))
	beta.CreationTimestamp = metav1.NewTime(time.Unix(100, 0))
	storage := newRegistryTestStorage(alpha, beta)
	for _, tc := range []struct {
		name   string
		cursor string
		want   string
		next   string
	}{
		{
			name: "oldest first",
			want: beta.Name,
			next: "com.example.obot/" + beta.Name,
		},
		{
			name:   "next page",
			cursor: "com.example.obot/" + beta.Name,
			want:   alpha.Name,
		},
		{
			name:   "unknown cursor starts at first page",
			cursor: "com.example.obot/missing",
			want:   beta.Name,
			next:   "com.example.obot/" + beta.Name,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := registryRequest(t, storage, nil, "/v0.1/servers?limit=1&cursor="+url.QueryEscape(tc.cursor))
			var list types.RegistryServerList
			decodeRegistryResponse(t, response, &list)
			if !slices.Equal(registryIDs(list), []string{tc.want}) || list.Metadata == nil || list.Metadata.Count != 1 || list.Metadata.NextCursor != tc.next {
				t.Fatalf("unexpected page: %+v", list)
			}
		})
	}
}

func TestRegistrySearchFullDescription(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix string
	}{
		{
			name:   "ASCII",
			prefix: strings.Repeat("a", 100),
		},
		{
			name:   "Unicode",
			prefix: strings.Repeat("界", 100),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := tc.prefix
			definition := registryDefinition("vmcp1long", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
			definition.Spec.Manifest.Description = prefix + " Forecasts beyond the response limit"
			storage := newRegistryTestStorage(definition)
			response := registryRequest(t, storage, nil, "/v0.1/servers?search=FORECASTS")
			var list types.RegistryServerList
			decodeRegistryResponse(t, response, &list)
			if !slices.Equal(registryIDs(list), []string{definition.Name}) {
				t.Fatalf("search beyond description limit: IDs = %v, want %s", registryIDs(list), definition.Name)
			}
			if got := list.Servers[0].Server.Description; got != prefix {
				t.Fatalf("response description = %q, want %q", got, prefix)
			}
		})
	}
}

func TestRegistryExcludesEmptyDefinitions(t *testing.T) {
	ready := registryDefinition("vmcp1ready", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	draft := registryDefinition("vmcp1draft", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	draft.Spec.Manifest.Components = nil
	draft.Spec.LegacySlug = "ms1draft"
	empty := registryDefinition("vmcp1empty", "", types.Subject{Type: types.SubjectTypeSelector, ID: "*"})
	empty.Spec.Manifest.Components = []types.VMCPComponent{}
	personal := registryDefinition("vmcp1personal", "user-1")
	personal.Spec.Manifest.Components = nil
	storage := newRegistryTestStorage(ready, draft, empty, personal)
	for _, tc := range []struct {
		name string
		user user.Info
	}{
		{
			name: "anonymous",
		},
		{
			name: "authenticated",
			user: &user.DefaultInfo{
				UID:    "user-1",
				Groups: []string{types.GroupAuthenticated},
			},
		},
		{
			name: "admin",
			user: &user.DefaultInfo{
				UID:    "user-1",
				Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := registryRequest(t, storage, tc.user, "/v0.1/servers?limit=1")
			var list types.RegistryServerList
			decodeRegistryResponse(t, response, &list)
			if !slices.Equal(registryIDs(list), []string{ready.Name}) || list.Metadata.NextCursor != "" {
				t.Fatalf("empty definitions must not affect results or pagination: %+v", list)
			}
			response = registryRequest(t, storage, tc.user, "/v0.1/servers?search=draft")
			decodeRegistryResponse(t, response, &list)
			if len(list.Servers) != 0 {
				t.Fatalf("empty draft matched search: %+v", list)
			}
			for _, id := range []string{draft.Name, draft.Spec.LegacySlug, empty.Name, personal.Name} {
				for _, suffix := range []string{"/versions", "/versions/latest"} {
					response := registryRequest(t, storage, tc.user, registryDetailPath(id)+suffix)
					if response.Code != http.StatusNotFound {
						t.Fatalf("%s%s: status = %d, want 404", id, suffix, response.Code)
					}
				}
			}
		})
	}
}

func registryDefinition(id, owner string, subjects ...types.Subject) *v1.VMCP {
	return &v1.VMCP{
		ObjectMeta: registryObjectMeta(id),
		Spec: v1.VMCPSpec{
			UserID: owner,
			Manifest: types.VMCPManifest{
				DisplayName: id,
				Components: []types.VMCPComponent{
					{ID: "component-1"},
				},
				Profiles: []types.VMCPProfile{
					{
						Name:     "access",
						Subjects: subjects,
					},
				},
			},
		},
	}
}

func newRegistryTestStorage(objects ...kclient.Object) *registryTestStorage {
	reader := clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.VMCP{}, "spec.legacySlug", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCP).Spec.LegacySlug}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.LegacySlug}
		}).
		Build()
	return &registryTestStorage{reader: reader}
}

func registryObjectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace}
}

func registryDetailPath(id string) string {
	return "/v0.1/servers/" + url.PathEscape("com.example.obot/"+id)
}

func registryRequest(t *testing.T, storage *registryTestStorage, u user.Info, path string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler("https://obot.example.com")
	mux := http.NewServeMux()
	for pattern, handler := range map[string]api.HandlerFunc{
		"GET /v0.1/servers":                                 h.ListServers,
		"GET /v0.1/servers/{serverName}/versions":           h.ListServerVersions,
		"GET /v0.1/servers/{serverName}/versions/{version}": h.GetServerVersion,
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			err := handler(api.Context{
				ResponseWriter: w,
				Request:        r,
				Storage:        storage,
				User:           u,
			})
			if err != nil {
				var httpErr *types.ErrHTTP
				if !errors.As(err, &httpErr) {
					t.Fatalf("%s: %v", path, err)
				}
				http.Error(w, httpErr.Message, httpErr.Code)
			}
		})
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func decodeRegistryResponse(t *testing.T, response *httptest.ResponseRecorder, out any) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), out); err != nil {
		t.Fatal(err)
	}
}

func registryIDs(list types.RegistryServerList) []string {
	ids := make([]string, 0, len(list.Servers))
	for _, server := range list.Servers {
		ids = append(ids, strings.TrimPrefix(server.Server.Name, "com.example.obot/"))
	}
	return ids
}

func listedRegistryServer(t *testing.T, list types.RegistryServerList, id string) types.RegistryServerResponse {
	t.Helper()
	for _, server := range list.Servers {
		if server.Server.Name == "com.example.obot/"+id {
			return server
		}
	}
	t.Fatalf("definition %s is missing from listing", id)
	return types.RegistryServerResponse{}
}

//nolint:revive
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auth"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/storage"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Context struct {
	http.ResponseWriter
	*http.Request
	Storage       storage.Client
	GatewayClient *gclient.Client
	User          user.Info
	APIBaseURL    string

	// LocalK8sClient is a kclient for the local Kubernetes cluster — the
	// cluster the obot pod runs in, where source Secrets for
	// secretBindings live. Nil on the docker backend
	LocalK8sClient client.Client

	// ObotNamespace is the Kubernetes namespace in which the obot server
	// runs; mcp.MergeBoundCreds reads source Secrets from here. Empty
	// when LocalK8sClient is nil.
	ObotNamespace string
}

type (
	HandlerFunc func(Context) error
	Middleware  func(HandlerFunc) HandlerFunc
)

func (r *Context) IsStreamRequested() bool {
	return r.Accepts("text/event-stream")
}

func (r *Context) Accepts(contentType string) bool {
	return slices.Contains(r.Request.Header.Values("Accept"), contentType)
}

func (r *Context) Read(obj any) error {
	data, err := r.Body()
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return io.EOF
	}
	return json.Unmarshal(data, obj)
}

type BodyOptions struct {
	// MaxBytes caps the body, applied to the compressed and decoded streams alike.
	MaxBytes int64
}

const defaultMaxBodyBytes int64 = 8 * 1024 * 1024

// maxDecoderWindowBytes caps the window a zstd frame may declare. The decoder
// allocates it from the frame header, before any output the decoded cap could
// bound.
const maxDecoderWindowBytes = 64 * 1024 * 1024

func (r *Context) Body(opts ...BodyOptions) (_ []byte, err error) {
	defer func() {
		if _, isMaxBytes := errors.AsType[*http.MaxBytesError](err); isMaxBytes {
			err = types.NewErrHTTP(http.StatusRequestEntityTooLarge, "request body too large")
		}
		_, _ = io.Copy(io.Discard, r.Request.Body)
	}()
	var opt BodyOptions
	for _, o := range opts {
		if o.MaxBytes > 0 {
			opt.MaxBytes = o.MaxBytes
		}
	}
	if opt.MaxBytes == 0 {
		opt.MaxBytes = defaultMaxBodyBytes
	}

	body := io.Reader(http.MaxBytesReader(r.ResponseWriter, r.Request.Body, opt.MaxBytes))

	// Device scans submit zstd. The decoded stream carries the same cap as the raw
	// one, read one byte over so a body at the cap is distinguishable from one
	// past it.
	var compressed bool
	if encoding := r.Request.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		if !strings.EqualFold(encoding, "zstd") {
			return nil, types.NewErrHTTP(http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported Content-Encoding %q", encoding))
		}
		zstdReader, zstdErr := zstd.NewReader(body,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderMaxMemory(maxDecoderWindowBytes))
		if zstdErr != nil {
			return nil, types.NewErrHTTP(http.StatusBadRequest, "unreadable zstd request body")
		}
		defer zstdReader.Close()
		compressed, body = true, io.LimitReader(zstdReader, opt.MaxBytes+1)
	}

	data, err := io.ReadAll(body)
	if err != nil {
		// zstd validates nothing until the first read, so a malformed body lands
		// here and would otherwise become a 500. A MaxBytesError is the cap, which
		// the deferred mapping turns into a 413.
		if _, isMaxBytes := errors.AsType[*http.MaxBytesError](err); compressed && !isMaxBytes {
			return nil, types.NewErrHTTP(http.StatusBadRequest, "malformed compressed request body")
		}
		return nil, err
	}
	if int64(len(data)) > opt.MaxBytes {
		return nil, types.NewErrHTTP(http.StatusRequestEntityTooLarge, "request body too large")
	}
	return data, nil
}

func (r *Context) WriteCreated(obj any) error {
	return r.write(obj, http.StatusCreated)
}

func (r *Context) Write(obj any) error {
	return r.write(obj, http.StatusOK)
}

func (r *Context) WriteCode(obj any, code int) error {
	return r.write(obj, code)
}

func (r *Context) write(obj any, code int) error {
	if data, ok := obj.([]byte); ok {
		r.ResponseWriter.Header().Set("Content-Type", "application/octet-stream")
		_, err := r.ResponseWriter.Write(data)
		return err
	} else if str, ok := obj.(string); ok {
		r.ResponseWriter.Header().Set("Content-Type", "text/plain")
		_, err := r.ResponseWriter.Write([]byte(str))
		return err
	}
	r.ResponseWriter.Header().Set("Content-Type", "application/json")
	r.WriteHeader(code)
	return json.NewEncoder(r.ResponseWriter).Encode(obj)
}

func (r *Context) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *Context) List(obj client.ObjectList, opts ...client.ListOption) error {
	namespace := r.Namespace()
	return r.Storage.List(r.Context(), obj, slices.Concat([]client.ListOption{
		&client.ListOptions{
			Namespace: namespace,
		},
	}, opts)...)
}

func (r *Context) Delete(obj client.Object) error {
	err := r.Storage.Delete(r.Context(), obj)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *Context) Get(obj client.Object, name string) error {
	namespace := r.Namespace()
	return r.Storage.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, obj)
}

func (r *Context) Create(obj client.Object) error {
	return r.Storage.Create(r.Context(), obj)
}

func (r *Context) Update(obj client.Object) error {
	return r.Storage.Update(r.Context(), obj)
}

func (r *Context) Namespace() string {
	return system.DefaultNamespace
}

func (r *Context) UserIsOwner() bool {
	return slices.Contains(r.User.GetGroups(), types.GroupOwner)
}

func (r *Context) UserIsAdmin() bool {
	return slices.Contains(r.User.GetGroups(), types.GroupAdmin)
}

func (r *Context) UserIsAuditor() bool {
	return slices.Contains(r.User.GetGroups(), types.GroupAuditor)
}

func (r *Context) UserCanImpersonate() bool {
	return slices.Contains(r.User.GetGroups(), types.GroupUserImpersonation)
}

func (r *Context) UserIsPowerUser() bool {
	return slices.Contains(r.User.GetGroups(), types.GroupPowerUser)
}

func (r *Context) UserIsPowerUserPlus() bool {
	return slices.Contains(r.User.GetGroups(), types.GroupPowerUserPlus)
}

func (r *Context) UserIsAuthenticated() bool {
	return slices.Contains(r.User.GetGroups(), types.GroupAuthenticated)
}

func (r *Context) UserID() uint {
	userID, err := strconv.ParseUint(r.User.GetUID(), 10, 64)
	if err != nil {
		return 0
	}
	return uint(userID)
}

func (r *Context) AuthProviderUserID() string {
	return auth.FirstExtraValue(r.User.GetExtra(), "auth_provider_user_id")
}

func (r *Context) AuthProviderNameAndNamespace() (string, string) {
	return auth.FirstExtraValue(r.User.GetExtra(), "auth_provider_name"),
		auth.FirstExtraValue(r.User.GetExtra(), "auth_provider_namespace")
}

func (r *Context) UserTimezone() string {
	return r.Request.Header.Get("X-Obot-User-Timezone")
}

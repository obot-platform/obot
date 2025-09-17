package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auth"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/storage"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Context struct {
	http.ResponseWriter
	*http.Request
	GPTClient     *gptscript.GPTScript
	Storage       storage.Client
	GatewayClient *gclient.Client
	User          user.Info
	APIBaseURL    string
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

func (r *Context) WriteEvents(events <-chan types.Progress) error {
	// Check if SSE is requested
	sendEvents := r.IsStreamRequested()

	sendJSON := r.Accepts("application/json")
	if sendEvents {
		r.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
		defer func() {
			_ = r.WriteDataEvent(EventClose{})
		}()
	}

	var (
		lastFlush time.Time
		toWrite   []types.Progress
	)
	if sendEvents {
		if _, err := r.ResponseWriter.Write([]byte("event: start\ndata: {}\n\n")); err != nil {
			return err
		}
		r.Flush()
	}
	for event := range events {
		if sendEvents {
			if err := r.WriteDataEvent(event); err != nil {
				return err
			}
		} else if sendJSON {
			toWrite = append(toWrite, event)
		} else {
			if err := r.Write([]byte(event.Content)); err != nil {
				return err
			}
			if lastFlush.IsZero() || time.Since(lastFlush) > 500*time.Millisecond {
				r.Flush()
				lastFlush = time.Now()
			}
		}
	}

	if sendJSON {
		return r.Write(map[string]any{
			"items": toWrite,
		})
	}

	return nil
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
	MaxBytes int64
}

func (r *Context) Body(opts ...BodyOptions) (_ []byte, err error) {
	defer func() {
		if maxErr := (*http.MaxBytesError)(nil); errors.As(err, &maxErr) {
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
		opt.MaxBytes = 8 * 1024 * 1024
	}
	return io.ReadAll(http.MaxBytesReader(r.ResponseWriter, r.Request.Body, opt.MaxBytes))
}

func (r *Context) WriteCreated(obj any) error {
	return r.write(obj, http.StatusCreated)
}

func (r *Context) Write(obj any) error {
	return r.write(obj, http.StatusOK)
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

type EventClose struct{}

func (r *Context) WriteDataEvent(obj any) error {
	if prg, ok := obj.(types.Progress); ok && prg.RunID != "" {
		if prg.RunComplete {
			if _, err := r.ResponseWriter.Write([]byte("id: " + prg.RunID + ":after\n")); err != nil {
				return err
			}
		} else {
			if _, err := r.ResponseWriter.Write([]byte("id: " + prg.RunID + "\n")); err != nil {
				return err
			}
		}
	}
	if _, ok := obj.(EventClose); ok {
		_, err := r.ResponseWriter.Write([]byte("event: close\ndata: {}\n\n"))
		return err
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	if _, err = r.ResponseWriter.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err = r.ResponseWriter.Write(data); err != nil {
		return err
	}
	if _, err = r.ResponseWriter.Write([]byte("\n\n")); err != nil {
		return err
	}
	r.Flush()
	return nil
}

func (r *Context) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func Watch[T client.Object](r Context, list client.ObjectList, opts ...client.ListOption) (<-chan T, error) {
	if err := r.List(list); err != nil {
		return nil, err
	}

	startList := list.DeepCopyObject().(client.ObjectList)

	watchOpts := append([]client.ListOption{&client.ListOptions{
		Namespace: r.Namespace(),
		Raw: &metav1.ListOptions{
			ResourceVersion: list.GetResourceVersion(),
		},
	}}, opts...)
	w, err := r.Storage.Watch(r.Context(), list, watchOpts...)
	if err != nil {
		return nil, err
	}

	resp := make(chan T)
	go func() {
		defer close(resp)
		defer w.Stop()

		_ = meta.EachListItem(startList, func(object runtime.Object) error {
			resp <- object.(T)
			return nil
		})

		for event := range w.ResultChan() {
			switch event.Type {
			case watch.Added, watch.Modified, watch.Deleted:
				resp <- event.Object.(T)
			}
		}
	}()

	return resp, nil
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

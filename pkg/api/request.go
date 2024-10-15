package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api/authz"
	"github.com/otto8-ai/otto8/pkg/storage"
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
	GPTClient *gptscript.GPTScript
	Storage   storage.Client
	User      user.Info
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
	}

	var (
		lastFlush time.Time
		toWrite   []types.Progress
	)
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
	return json.Unmarshal(data, obj)
}

func (r *Context) Body() ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Request.Body, 1<<20))
}

func (r *Context) Write(obj any) error {
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
	return json.NewEncoder(r.ResponseWriter).Encode(obj)
}

func (r *Context) WriteDataEvent(obj any) error {
	if prg, ok := obj.(*types.Progress); ok && prg.RunID != "" {
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

func Watch[T client.Object](r Context, list client.ObjectList) (<-chan T, error) {
	if err := r.List(list); err != nil {
		return nil, err
	}

	startList := list.DeepCopyObject().(client.ObjectList)

	w, err := r.Storage.Watch(r.Request.Context(), list, &client.ListOptions{
		Namespace: r.Namespace(),
		Raw: &metav1.ListOptions{
			ResourceVersion: list.GetResourceVersion(),
		},
	})
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

func (r *Context) List(obj client.ObjectList) error {
	namespace := r.Namespace()
	return r.Storage.List(r.Request.Context(), obj, &client.ListOptions{
		Namespace: namespace,
	})
}

func (r *Context) Delete(obj client.Object) error {
	err := r.Storage.Delete(r.Request.Context(), obj)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *Context) Get(obj client.Object, name string) error {
	namespace := r.Namespace()
	err := r.Storage.Get(r.Request.Context(), client.ObjectKey{Namespace: namespace, Name: name}, obj)
	if apierrors.IsNotFound(err) {
		gvk, _ := r.Storage.GroupVersionKindFor(obj)
		return types.NewErrHttp(http.StatusNotFound, fmt.Sprintf("%s %s not found", strings.ToLower(gvk.Kind), name))
	}
	return err
}

func (r *Context) Create(obj client.Object) error {
	return r.Storage.Create(r.Request.Context(), obj)
}

func (r *Context) Update(obj client.Object) error {
	return r.Storage.Update(r.Request.Context(), obj)
}

func (r *Context) Namespace() string {
	return "default"
}

func (r *Context) UserIsAdmin() bool {
	return slices.Contains(r.User.GetGroups(), authz.AdminGroup)
}

func (r *Context) UserIsAuthenticated() bool {
	return slices.Contains(r.User.GetGroups(), authz.AuthenticatedGroup)
}

func (r *Context) UserID() uint {
	userID, err := strconv.ParseUint(r.User.GetUID(), 10, 64)
	if err != nil {
		return 0
	}
	return uint(userID)
}

func (r *Context) AuthProviderID() uint {
	extraAuthProvider := r.User.GetExtra()["auth_provider_id"]
	if len(extraAuthProvider) == 0 {
		return 0
	}
	authProviderID, err := strconv.ParseUint(extraAuthProvider[0], 10, 64)
	if err != nil {
		return 0
	}

	return uint(authProviderID)
}

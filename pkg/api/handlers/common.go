package handlers

import (
	"reflect"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

func MetadataFrom(obj kclient.Object, linkKV ...string) types.Metadata {
	m := types.Metadata{
		ID:      obj.GetName(),
		Created: *types.NewTime(obj.GetCreationTimestamp().Time),
		Links:   map[string]string{},
		Type:    strings.ToLower(reflect.TypeOf(obj).Elem().Name()),
	}
	if delTime := obj.GetDeletionTimestamp(); delTime != nil {
		m.Deleted = types.NewTime(delTime.Time)
	}
	for i := 0; i < len(linkKV); i += 2 {
		m.Links[linkKV[i]] = linkKV[i+1]
	}
	return m
}

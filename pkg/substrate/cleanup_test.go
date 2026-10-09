package substrate

import (
	"context"
	"testing"
	"time"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type cleanupControl struct {
	pb.ControlClient
	templates []*pb.ActorTemplate
	calls     []string
	borrowed  bool
}

func (c *cleanupControl) DeleteActor(context.Context, *pb.DeleteActorRequest, ...grpc.CallOption) (*pb.Actor, error) {
	c.calls = append(c.calls, "actor")
	return nil, nil
}
func (c *cleanupControl) GetActor(context.Context, *pb.GetActorRequest, ...grpc.CallOption) (*pb.Actor, error) {
	return nil, status.Error(codes.NotFound, "gone")
}
func (c *cleanupControl) GetTag(context.Context, *pb.GetTagRequest, ...grpc.CallOption) (*pb.Tag, error) {
	return &pb.Tag{Status: &pb.TagStatus{Snapshot: &pb.ExternalSnapshot{SnapshotUri: "s3://tag"}}}, nil
}
func (c *cleanupControl) ListActors(_ context.Context, request *pb.ListActorsRequest, _ ...grpc.CallOption) (*pb.ListActorsResponse, error) {
	if request.PageToken == "" {
		return &pb.ListActorsResponse{NextPageToken: "second"}, nil
	}
	if c.borrowed {
		return &pb.ListActorsResponse{Actors: []*pb.Actor{{Status: &pb.ActorStatus{ExternalSnapshot: &pb.ExternalSnapshot{SnapshotUri: "s3://tag"}}}}}, nil
	}
	return &pb.ListActorsResponse{}, nil
}
func (c *cleanupControl) DeleteTag(context.Context, *pb.DeleteTagRequest, ...grpc.CallOption) (*pb.Tag, error) {
	c.calls = append(c.calls, "tag")
	return nil, nil
}
func (c *cleanupControl) DeleteActorTemplate(context.Context, *pb.DeleteActorTemplateRequest, ...grpc.CallOption) (*pb.ActorTemplate, error) {
	c.calls = append(c.calls, "template")
	return nil, nil
}

func TestCleanupProtectsBorrowedCheckpoint(t *testing.T) {
	control := &cleanupControl{borrowed: true}
	client := Client{Control: control}
	ref := &pb.ObjectRef{Atespace: "obot-test", Name: "acr-test"}
	require.ErrorContains(t, client.DeleteExecution(t.Context(), ref), "dependent actor checkpoint")
	require.Equal(t, []string{"actor"}, control.calls)
	control.calls = nil
	control.borrowed = false
	require.NoError(t, client.DeleteExecution(t.Context(), ref))
	require.Equal(t, []string{"actor", "tag", "template"}, control.calls)
}

func (c *cleanupControl) ListActorTemplates(_ context.Context, request *pb.ListActorTemplatesRequest, _ ...grpc.CallOption) (*pb.ListActorTemplatesResponse, error) {
	if request.PageToken == "" {
		return &pb.ListActorTemplatesResponse{NextPageToken: "second"}, nil
	}
	return &pb.ListActorTemplatesResponse{ActorTemplates: c.templates}, nil
}

func TestOrphanCleanupKeepsLiveAndRecentTemplates(t *testing.T) {
	var instance v1.AgentInstance
	instance.Spec.UserID = "owner"
	instance.Name = "acr-live"
	ref := Ref(&instance)
	template := func(name string, created time.Time) *pb.ActorTemplate {
		return &pb.ActorTemplate{
			Metadata:       &pb.ResourceMetadata{Atespace: ref.Atespace, Name: name, CreateTime: timestamppb.New(created)},
			WorkerSelector: &pb.Selector{MatchLabels: map[string]string{environmentLabel: "environment"}},
		}
	}
	old := time.Now().Add(-time.Hour)
	foreign := template("acr-foreign", old)
	foreign.WorkerSelector = nil
	control := &cleanupControl{templates: []*pb.ActorTemplate{template("acr-live", old), template("acr-recent", time.Now()), foreign, template("acr-orphan", old)}}
	client := Client{Control: control}
	require.NoError(t, client.CollectOrphans(t.Context(), []v1.AgentInstance{instance}, client.DeleteExecution))
	require.Equal(t, []string{"actor", "tag", "template"}, control.calls)

	control.calls = nil
	control.borrowed = true
	control.templates = append(control.templates, template("acr-second-orphan", old))
	require.ErrorContains(t, client.CollectOrphans(t.Context(), []v1.AgentInstance{instance}, client.DeleteExecution), "dependent actor checkpoint")
	require.Equal(t, []string{"actor", "actor"}, control.calls, "a blocked source must not prevent cleanup of its orphaned dependents")
}

package substrate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (c *Client) actors(ctx context.Context) ([]*pb.Actor, error) {
	var actors []*pb.Actor
	request := &pb.ListActorsRequest{PageSize: 1000}
	for {
		page, err := c.Control.ListActors(ctx, request)
		if err != nil {
			return nil, err
		}
		actors = append(actors, page.Actors...)
		request.PageToken = page.NextPageToken
		if request.PageToken == "" {
			return actors, nil
		}
	}
}

func (c *Client) DeleteExecution(ctx context.Context, ref *pb.ObjectRef) error {
	_, err := c.Control.DeleteActor(ctx, &pb.DeleteActorRequest{Actor: ref, AnyState: true})
	if err != nil && status.Code(err) != codes.NotFound {
		return err
	}
	if _, err = c.Control.GetActor(ctx, &pb.GetActorRequest{Actor: ref}); status.Code(err) != codes.NotFound {
		if err != nil {
			return err
		}
		return fmt.Errorf("waiting for actor deletion")
	}
	// A cloned actor borrows its source tag until its first successful suspend.
	tag, err := c.Control.GetTag(ctx, &pb.GetTagRequest{Tag: ref})
	if err != nil && status.Code(err) != codes.NotFound {
		return err
	}
	if err == nil {
		actors, err := c.actors(ctx)
		if err != nil {
			return err
		}
		snapshot := tag.GetStatus().GetSnapshot().GetSnapshotUri()
		for _, actor := range actors {
			if snapshot != "" && actor.GetStatus().GetExternalSnapshot().GetSnapshotUri() == snapshot {
				return fmt.Errorf("waiting for dependent actor checkpoint")
			}
		}
		if _, err := c.Control.DeleteTag(ctx, &pb.DeleteTagRequest{Tag: ref}); err != nil && status.Code(err) != codes.NotFound {
			return err
		}
	}
	_, err = c.Control.DeleteActorTemplate(ctx, &pb.DeleteActorTemplateRequest{ActorTemplate: ref})
	if status.Code(err) == codes.NotFound {
		return nil
	}
	return err
}

// CollectOrphans recovers resources left behind after interrupted provisioning or
// database loss. Only Obot execution templates are considered; golden actors are
// collected by Substrate when their owning template is deleted.
func (c *Client) CollectOrphans(ctx context.Context, instances []v1.AgentInstance, deleteExecution func(context.Context, *pb.ObjectRef) error) error {
	live := map[string]bool{}
	for i := range instances {
		ref := Ref(&instances[i])
		live[ref.Atespace+"/"+ref.Name] = true
		if previous := instances[i].Spec.ClaudePrevious; previous != "" {
			live[ref.Atespace+"/"+previous] = true
		}
	}
	request := &pb.ListActorTemplatesRequest{PageSize: 1000}
	var templates []*pb.ActorTemplate
	for {
		page, err := c.Control.ListActorTemplates(ctx, request)
		if err != nil {
			return err
		}
		templates = append(templates, page.ActorTemplates...)
		request.PageToken = page.NextPageToken
		if request.PageToken == "" {
			break
		}
	}
	var cleanupErr error
	for _, template := range templates {
		meta := template.GetMetadata()
		labels := template.GetWorkerSelector().GetMatchLabels()
		if !strings.HasPrefix(meta.GetAtespace(), "obot-") || !strings.HasPrefix(meta.GetName(), "acr-") ||
			(labels[environmentLabel] == "" && labels["obot.ai/agent-runtime"] != "claude-code") {
			continue
		}
		if live[meta.Atespace+"/"+meta.Name] || meta.GetCreateTime() == nil || time.Since(meta.CreateTime.AsTime()) < 15*time.Minute {
			continue
		}
		if err := deleteExecution(ctx, &pb.ObjectRef{Atespace: meta.Atespace, Name: meta.Name}); err != nil {
			// A source tag may be borrowed by another orphan later in this list.
			// Process that orphan too, then retry the source on the next sweep.
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

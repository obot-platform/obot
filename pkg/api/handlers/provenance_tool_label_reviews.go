package handlers

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type toolLabelReview struct {
	toolLabelResource
	DisplayName string    `json:"displayName"`
	ToolCount   int       `json:"toolCount"`
	GeneratedAt time.Time `json:"generatedAt"`
}

type toolLabelReviews struct {
	Items []toolLabelReview `json:"items"`
	Total int               `json:"total"`
}

// ListToolLabelReviews groups current proposals into resource reviews for the
// attention queue. It reads existing proposals and never calls a model.
func (h *ProvenanceHandler) ListToolLabelReviews(req api.Context) error {
	labels, err := h.engine.Store().ListToolLabels(req.Context())
	if err != nil {
		return err
	}
	resources := map[toolLabelResource]struct{}{}
	for _, label := range labels {
		if label.Proposal == nil || label.Proposal.SchemaHash == "" {
			continue
		}
		kind, id, ok := strings.Cut(label.Server, ":")
		if !ok || id == "" || (kind != "catalog-entry" && kind != "vmcp") {
			continue
		}
		resources[toolLabelResource{ResourceType: kind, ResourceID: id}] = struct{}{}
	}
	out := toolLabelReviews{Items: []toolLabelReview{}}
	for resource := range resources {
		inventory, err := h.toolLabelInventoryFromLabels(req, resource, labels)
		if err != nil {
			var httpErr *types.ErrHTTP
			if apierrors.IsNotFound(err) || (errors.As(err, &httpErr) && httpErr.Code == 404) {
				continue
			}
			if errors.As(err, &httpErr) && httpErr.Code == 400 {
				// Invalid inventories need repair in label management before
				// their proposals can be reviewed. They are not approval items.
				continue
			}
			return err
		}
		review := toolLabelReview{toolLabelResource: resource, DisplayName: inventory.DisplayName}
		for _, item := range inventory.Items {
			if item.Proposal == nil || item.Proposal.SchemaHash == "" || item.Stale {
				continue
			}
			review.ToolCount++
			if item.Proposal.GeneratedAt.After(review.GeneratedAt) {
				review.GeneratedAt = item.Proposal.GeneratedAt
			}
		}
		if review.ToolCount > 0 {
			out.Items = append(out.Items, review)
		}
	}
	slices.SortFunc(out.Items, func(a, b toolLabelReview) int {
		if cmp := b.GeneratedAt.Compare(a.GeneratedAt); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.DisplayName, b.DisplayName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ResourceType+":"+a.ResourceID, b.ResourceType+":"+b.ResourceID)
	})
	out.Total = len(out.Items)
	return req.Write(out)
}

package groupref

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// WriteNewSubjects runs write, which saves subjects in place of previous, and returns what write returns. The group
// subjects that subjects adds must reach groups of a SCIM connection's auth provider: a group of the provider must
// have the ID, and must not be being deleted because nothing referenced it. Such a reference would match nobody, and
// no pushed group could bind to it. Subjects that previous already has are left alone, so an old reference to a
// missing group does not block unrelated edits.
//
// The write is recorded while it runs, so a deletion of unreferenced groups either waits for it and reads the new
// references, or marks its groups first and write is refused. A refusal is a bad request that names the provider by
// its display name, which is read from storage.
func WriteNewSubjects(ctx context.Context, gateway *gclient.Client, storage kclient.Reader, subjects, previous []types.Subject, write func() error) error {
	existing := make(map[string]struct{}, len(previous))
	for _, subject := range previous {
		if subject.Type == types.SubjectTypeGroup {
			existing[subject.ID] = struct{}{}
		}
	}

	var added []string
	for _, subject := range subjects {
		if _, ok := existing[subject.ID]; subject.Type == types.SubjectTypeGroup && !ok {
			added = append(added, subject.ID)
		}
	}
	return WriteNewGroups(ctx, gateway, storage, added, write)
}

// WriteNewGroups runs write, which saves new references to groupIDs, as WriteNewSubjects does.
func WriteNewGroups(ctx context.Context, gateway *gclient.Client, storage kclient.Reader, groupIDs []string, write func() error) error {
	err := gateway.WithNewSCIMGroupReferences(ctx, groupIDs, write)
	refErr, ok := errors.AsType[*gclient.SCIMGroupReferenceError](err)
	if !ok {
		return err
	}

	name := refErr.AuthProviderName
	var authProvider v1.AuthProvider
	if err := storage.Get(ctx, kclient.ObjectKey{Namespace: refErr.AuthProviderNamespace, Name: refErr.AuthProviderName}, &authProvider); err == nil {
		name = cmp.Or(authProvider.Spec.Name, name)
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get auth provider %q: %w", refErr.AuthProviderName, err)
	}

	var problems []string
	switch len(refErr.Missing) {
	case 0:
	case 1:
		problems = append(problems, fmt.Sprintf("no %s group has the ID %s. %s provisions its groups through SCIM, so push the group from %s first, or choose an existing group",
			name, refErr.Missing[0], name, name))
	default:
		problems = append(problems, fmt.Sprintf("no %s groups have the IDs %s. %s provisions its groups through SCIM, so push the groups from %s first, or choose existing groups",
			name, strings.Join(refErr.Missing, ", "), name, name))
	}
	switch len(refErr.PendingDeletion) {
	case 0:
	case 1:
		problems = append(problems, fmt.Sprintf("the %s group %s is being deleted because nothing referenced it. Choose another group, or try again once the deletion finishes",
			name, refErr.PendingDeletion[0]))
	default:
		problems = append(problems, fmt.Sprintf("the %s groups %s are being deleted because nothing referenced them. Choose other groups, or try again once the deletion finishes",
			name, strings.Join(refErr.PendingDeletion, ", ")))
	}
	return types.NewErrBadRequest("%s", strings.Join(problems, "; "))
}

// VMCPProfileSubjects returns the subjects of every profile of a virtual MCP server.
func VMCPProfileSubjects(manifest types.VMCPManifest) []types.Subject {
	var subjects []types.Subject
	for _, profile := range manifest.Profiles {
		subjects = append(subjects, profile.Subjects...)
	}
	return subjects
}

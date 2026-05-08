package publishedartifact

import (
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

func CanAccess(artifact *v1.PublishedArtifact, requester user.Info, isAdmin bool) bool {
	if isAdmin || artifact.Spec.AuthorID == requester.GetUID() {
		return true
	}
	for _, version := range artifact.Status.Versions {
		if SubjectsContainUser(version.Subjects, requester) {
			return true
		}
	}
	return false
}

func CanAccessVersion(artifact *v1.PublishedArtifact, version int, requester user.Info, isAdmin bool) bool {
	if isAdmin || artifact.Spec.AuthorID == requester.GetUID() {
		return VersionEntry(artifact, version) != nil
	}
	return SubjectsContainUser(VersionSubjects(artifact, version), requester)
}

func DefaultDownloadVersion(artifact *v1.PublishedArtifact, requester user.Info, isAdmin bool) int {
	if isAdmin || artifact.Spec.AuthorID == requester.GetUID() {
		return artifact.Spec.LatestVersion
	}

	var latestVisible int
	for _, version := range artifact.Status.Versions {
		if SubjectsContainUser(version.Subjects, requester) && version.Version > latestVisible {
			latestVisible = version.Version
		}
	}
	return latestVisible
}

func VersionEntry(artifact *v1.PublishedArtifact, version int) *types.PublishedArtifactVersionEntry {
	for _, v := range artifact.Status.Versions {
		if v.Version == version {
			return &v
		}
	}
	return nil
}

func VersionSubjects(artifact *v1.PublishedArtifact, version int) []types.Subject {
	entry := VersionEntry(artifact, version)
	if entry == nil {
		return nil
	}
	return entry.Subjects[:]
}

func SubjectsContainUser(subjects []types.Subject, requester user.Info) bool {
	userID := requester.GetUID()
	groups := authGroupSet(requester)
	for _, subject := range subjects {
		switch subject.Type {
		case types.SubjectTypeUser:
			if subject.ID == userID {
				return true
			}
		case types.SubjectTypeGroup:
			if _, ok := groups[subject.ID]; ok {
				return true
			}
		case types.SubjectTypeSelector:
			if subject.ID == "*" {
				return true
			}
		}
	}

	return false
}

func authGroupSet(requester user.Info) map[string]struct{} {
	providerGroups := requester.GetExtra()["auth_provider_groups"]
	result := make(map[string]struct{}, len(providerGroups))
	for _, group := range providerGroups {
		result[group] = struct{}{}
	}
	return result
}

package skillrepository

import (
	"context"

	gitpkg "github.com/obot-platform/obot/pkg/git"
)

type gitRepositoryFetcher struct{}

func newGitRepositoryFetcher() *gitRepositoryFetcher {
	return &gitRepositoryFetcher{}
}

func (f *gitRepositoryFetcher) Fetch(ctx context.Context, repoURL, token, ref string) (*fetchedRepository, error) {
	dir, commitSHA, cleanup, err := gitpkg.Clone(ctx, repoURL, token, ref)
	if err != nil {
		return nil, err
	}
	return &fetchedRepository{
		RepoRoot:  dir,
		CommitSHA: commitSHA,
		cleanup:   cleanup,
	}, nil
}

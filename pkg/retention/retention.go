package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager struct {
	retentionPolicy time.Duration
	runFrequency    time.Duration
	storageClient   storage.Client
}

func NewRetentionManager(retentionPolicy, runFrequency string, storageClient storage.Client) (*Manager, error) {
	retentionPolicyDuration, err := time.ParseDuration(retentionPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to parse retention policy: %w", err)
	}

	runFrequencyDuration, err := time.ParseDuration(runFrequency)
	if err != nil {
		return nil, fmt.Errorf("failed to parse run frequency: %w", err)
	}

	return &Manager{
		retentionPolicy: retentionPolicyDuration,
		runFrequency:    runFrequencyDuration,
		storageClient:   storageClient,
	}, nil
}

func (rm *Manager) Start(ctx context.Context) {
	logger := logger.New("retention")
	logger.Infof("starting retention manager with policy %s and run frequency %s", rm.retentionPolicy, rm.runFrequency)

	ticker := time.NewTicker(rm.runFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.runRetention(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// runRetention deletes threads and obots (projects) that haven't been used in the retention policy.
// It first deletes any normal chat threads (non-project, non-system tasks) that haven't been used in the retention policy.
// It then looks for any projects that were created more than the retention policy ago and deletes them if they have no
// remaining threads (meaning they haven't been used in the retention policy).
func (rm *Manager) runRetention(ctx context.Context) {
	logger := logger.New("retention")

	nonProjectNonSystemTaskSelector := client.MatchingFields{
		"spec.project":    "false",
		"spec.systemTask": "false",
	}

	var threads v1.ThreadList
	if err := rm.storageClient.List(ctx, &threads, nonProjectNonSystemTaskSelector); err != nil {
		logger.Errorf("failed to list threads: %v", err)
		return
	}

	// Loop through all of the individual chat threads and delete them if they haven't been used in the retention policy.
	for _, thread := range threads.Items {
		if thread.Status.LastRunName == "" {
			continue
		}

		var lastRun v1.Run
		logger.Debugf("getting last run %s for thread %s", thread.Status.LastRunName, thread.Name)
		if err := rm.storageClient.Get(ctx, client.ObjectKey{Namespace: thread.Namespace, Name: thread.Status.LastRunName}, &lastRun); err != nil {
			logger.Errorf("failed to get last run %s for thread %s: %v", thread.Status.LastRunName, thread.Name, err)
			continue
		}

		if time.Since(lastRun.CreationTimestamp.Time) > rm.retentionPolicy {
			logger.Infof("deleting thread %s", thread.Name)
			if err := rm.storageClient.Delete(ctx, &thread); err != nil {
				logger.Errorf("failed to delete thread %s: %v", thread.Name, err)
			}
		}
	}

	projectSelector := client.MatchingFields{
		"spec.project":    "true",
		"spec.systemTask": "false",
	}

	var projects v1.ThreadList
	if err := rm.storageClient.List(ctx, &projects, projectSelector); err != nil {
		logger.Errorf("failed to list projects: %v", err)
		return
	}

	logger.Debugf("found %d projects", len(projects.Items))

	// Loop through all of the project threads and delete them if they have zero threads and were created more than the retention policy ago.
	for _, project := range projects.Items {
		if time.Since(project.CreationTimestamp.Time) < rm.retentionPolicy {
			logger.Debugf("project %s was created %s ago, skipping", project.Name, time.Since(project.CreationTimestamp.Time))
			continue
		}

		// List all of the threads in the project.
		threadsInProjectSelector := client.MatchingFields{
			"spec.project":          "false",
			"spec.parentThreadName": project.Name,
			"spec.systemTask":       "false",
		}

		var threads v1.ThreadList
		if err := rm.storageClient.List(ctx, &threads, threadsInProjectSelector); err != nil {
			logger.Errorf("failed to list threads for project %s: %v", project.Name, err)
			continue
		}

		if len(threads.Items) == 0 {
			logger.Infof("deleting project %s", project.Name)
			if err := rm.storageClient.Delete(ctx, &project); err != nil {
				logger.Errorf("failed to delete project %s: %v", project.Name, err)
			}
		} else {
			logger.Debugf("project %s has %d threads, skipping", project.Name, len(threads.Items))
		}
	}
}

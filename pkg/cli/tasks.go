package cli

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

type Tasks struct {
	root        *Obot
	Quiet       bool   `usage:"Only print IDs of tasks" short:"q"`
	Wide        bool   `usage:"Print more information" short:"w"`
	Output      string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
	ThreadID    string `usage:"Filter tasks by Thread ID" short:"t"`
	AssistantID string `usage:"Filter tasks by Assistant ID" short:"a"`
	ProjectID   string `usage:"Filter tasks by Project (Obot) ID" short:"p"`
	NoRuns      bool   `usage:"Don't fetch run counts (faster)" short:"n"`
	MaxTasks    int    `usage:"Maximum number of tasks to process per thread" default:"10"`
	All         bool   `usage:"List all tasks (admin only)" short:"A"`
}

func (l *Tasks) Customize(cmd *cobra.Command) {
	cmd.Use = "tasks [flags] [TASK_ID...]"
	cmd.Aliases = []string{"task", "ts"}
}

func (l *Tasks) Run(cmd *cobra.Command, args []string) error {
	debugPrint := func(format string, args ...interface{}) {
		if l.root.Debug {
			fmt.Fprintf(cmd.OutOrStdout(), format+"\n", args...)
		}
	}

	ctx := cmd.Context()
	start := time.Now()

	// If ProjectID is provided but AssistantID is not, look up the assistant ID
	if l.ProjectID != "" && l.AssistantID == "" {
		debugPrint("Looking up assistant ID for project %s", l.ProjectID)
		project, err := l.root.Client.GetProject(ctx, l.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to get project %s: %w", l.ProjectID, err)
		}
		l.AssistantID = project.AssistantID
		debugPrint("Found assistant ID %s for project %s", l.AssistantID, l.ProjectID)
	}

	var (
		allTasks types.TaskList
	)

	// 1. First fetch projects to know what we have access to
	debugPrint("Fetching projects...")
	startProjects := time.Now()

	projects, err := l.root.Client.ListProjects(ctx, apiclient.ListProjectsOptions{
		All: l.All,
	})
	if err != nil {
		debugPrint("Error listing projects: %v", err)
		return fmt.Errorf("failed to list projects: %w", err)
	}
	debugPrint("Fetched %d projects in %.2f seconds", len(projects.Items), time.Since(startProjects).Seconds())

	// Build lookup maps for quick access
	projectMap := make(map[string]types.Project)
	for _, project := range projects.Items {
		projectMap[project.ID] = project
	}

	// If specific project is requested, only get tasks for that project
	if l.ProjectID != "" && l.AssistantID != "" {
		debugPrint("Using specific project %s with assistant %s", l.ProjectID, l.AssistantID)
		tasks, err := l.root.Client.ListProjectTasks(ctx, l.AssistantID, l.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to list tasks for project %s: %w", l.ProjectID, err)
		}
		allTasks = tasks
	} else {
		// Otherwise aggregate tasks from all accessible projects
		taskChan := make(chan types.Task)
		errChan := make(chan error, len(projects.Items))
		doneChan := make(chan struct{})

		// Keep track of how many goroutines we've started
		var wg sync.WaitGroup

		// Start a collector to aggregate all tasks
		go func() {
			for task := range taskChan {
				allTasks.Items = append(allTasks.Items, task)
			}
			close(doneChan)
		}()

		// Browser-like connection limit
		// Use a semaphore to limit concurrent API calls
		sem := make(chan struct{}, 8)

		// Filter out projects without assistantID to avoid unnecessary API calls
		var validProjects int
		for _, project := range projects.Items {
			if project.AssistantID == "" {
				debugPrint("Skipping project %s (no assistant ID)", project.ID)
				continue
			}
			validProjects++

			wg.Add(1)
			go func(p types.Project) {
				defer wg.Done()

				// Acquire semaphore slot
				sem <- struct{}{}
				defer func() { <-sem }() // Release slot when done

				startTask := time.Now()
				debugPrint("Fetching tasks for project %s (assistant: %s)", p.ID, p.AssistantID)

				tasks, err := l.root.Client.ListProjectTasks(ctx, p.AssistantID, p.ID)
				if err != nil {
					debugPrint("Error listing tasks for project %s: %v", p.ID, err)
					errChan <- err
					return
				}

				for _, task := range tasks.Items {
					taskChan <- task
				}

				debugPrint("Fetched %d tasks for project %s in %.2f seconds",
					len(tasks.Items), p.ID, time.Since(startTask).Seconds())
			}(project)
		}

		wg.Wait()
		close(taskChan)
		close(errChan)

		var errs []error
		for err := range errChan {
			errs = append(errs, err)
		}

		// Wait for collector to finish
		<-doneChan

		if len(errs) > 0 {
			debugPrint("Warning: Some project tasks could not be fetched (%d errors):", len(errs))
			for i, err := range errs {
				debugPrint("  Error %d: %v", i+1, err)
			}
		}

		debugPrint("Fetched a total of %d tasks from %d projects in %.2f seconds",
			len(allTasks.Items), validProjects, time.Since(startProjects).Seconds())
	}

	// Filter tasks if specific IDs were provided
	var tasksList types.TaskList
	if len(args) > 0 {
		wantedTaskIDs := make(map[string]bool)
		for _, arg := range args {
			wantedTaskIDs[arg] = true
		}

		for _, task := range allTasks.Items {
			if wantedTaskIDs[task.ID] {
				tasksList.Items = append(tasksList.Items, task)
				delete(wantedTaskIDs, task.ID)
			}
		}

		// Report any tasks we couldn't find
		for taskID := range wantedTaskIDs {
			debugPrint("Task ID not found: %s", taskID)
		}
	} else {
		// Use all tasks if no filter was applied
		tasksList = allTasks
	}

	// Sort tasks by creation time (newest first)
	sort.Slice(tasksList.Items, func(i, j int) bool {
		return tasksList.Items[i].Created.Time.After(tasksList.Items[j].Created.Time)
	})

	// Handle different output formats
	if ok, err := output(l.Output, tasksList); ok || err != nil {
		return err
	}

	if l.Quiet {
		for _, task := range tasksList.Items {
			fmt.Println(task.ID)
		}
		return nil
	}

	// Fetch run counts for each task if needed
	runCounts := make(map[string]int, len(tasksList.Items))
	if !l.NoRuns {
		debugPrint("Fetching run counts for tasks...")
		startRunCounts := time.Now()

		// Use semaphore to limit concurrent API calls
		sem := make(chan struct{}, 8)
		var wg sync.WaitGroup
		var mu sync.Mutex // For thread-safe access to the runCounts map

		for _, task := range tasksList.Items {
			if task.ProjectID == "" || projectMap[task.ProjectID].AssistantID == "" {
				continue
			}

			wg.Add(1)
			go func(task types.Task) {
				defer wg.Done()

				// Acquire semaphore slot
				sem <- struct{}{}
				defer func() { <-sem }() // Release slot when done

				assistantID := projectMap[task.ProjectID].AssistantID

				runs, err := l.root.Client.ListTaskRuns(ctx, task.ID, apiclient.ListTaskRunsOptions{
					AssistantID: assistantID,
					ProjectID:   task.ProjectID,
				})

				if err != nil {
					debugPrint("Error fetching runs for task %s: %v", task.ID, err)
					return
				}

				mu.Lock()
				runCounts[task.ID] = len(runs.Items)
				mu.Unlock()
			}(task)
		}

		wg.Wait()
		debugPrint("Fetched run counts in %.2f seconds", time.Since(startRunCounts).Seconds())
	}

	// Create a nice table output
	var w *table
	if l.NoRuns {
		w = newTable("ID", "NAME", "OBOT", "OBOT ID", "DESCRIPTION", "CREATED")
	} else {
		w = newTable("ID", "NAME", "OBOT", "OBOT ID", "RUNS", "DESCRIPTION", "CREATED")
	}

	for _, task := range tasksList.Items {
		// Determine associated obot name
		obotName := task.ProjectID
		obotID := task.ProjectID
		if project, exists := projectMap[task.ProjectID]; exists {
			obotName = project.Name
			if obotName == "" {
				obotName = project.ID
			}
		}

		if l.NoRuns {
			w.WriteRow(
				task.ID,
				task.Name,
				obotName,
				obotID,
				truncate(task.Description, l.Wide),
				humanize.Time(task.Created.Time),
			)
		} else {
			runCount := runCounts[task.ID]
			w.WriteRow(
				task.ID,
				task.Name,
				obotName,
				obotID,
				fmt.Sprintf("%d", runCount),
				truncate(task.Description, l.Wide),
				humanize.Time(task.Created.Time),
			)
		}
	}

	debugPrint("Total execution time: %.2f seconds", time.Since(start).Seconds())

	return w.Err()
}

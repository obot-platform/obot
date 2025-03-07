<script lang="ts">
	import { ChatService, type Project, type Task, type TaskRun } from '$lib/services';
	import { onMount } from 'svelte';
	import { formatTime } from '$lib/time.js';
	import { overflowToolTip } from '$lib/actions/overflow';

	interface Props {
		project: Project;
		task: Task;
	}

	let { project, task }: Props = $props();
	let runs = $state<TaskRun[]>();

	onMount(async () => {
		runs = (await ChatService.listTaskRuns(project.assistantID, project.id, task.id)).items;
	});
</script>

<ul>
	{#each runs ?? [] as run}
		<li>
			<button>
				<span use:overflowToolTip>
					{run.id}: {formatTime(run.created)} asdf asdf asdf asdf
				</span>
			</button>
		</li>
	{/each}
</ul>

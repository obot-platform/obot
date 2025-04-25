<script lang="ts">
	import { type Project, ChatService, type Assistant } from '$lib/services';
	import { onDestroy, onMount } from 'svelte';
	import Obot from '$lib/components/Obot.svelte';
	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable(), assistant }: Props = $props();

	let projectSaved = '';
	let timer: number = 0;

	async function updateProject() {
		if (JSON.stringify(project) === projectSaved) {
			return;
		}
		const oldProject = JSON.stringify(project);
		const newProject = await ChatService.updateProject(project);
		projectSaved = JSON.stringify(newProject);
		if (oldProject === JSON.stringify(project)) {
			project = newProject;
		}
	}

	async function loadProject() {
		// tools = (await ChatService.listTools(project.assistantID, project.id)).items;
		projectSaved = JSON.stringify(project);
	}

	onDestroy(() => clearInterval(timer));

	onMount(() => {
		loadProject().then(() => {
			timer = setInterval(updateProject, 1000);
		});
	});
</script>

<div class="bg-surface1 flex size-full flex-col">
	<div class="flex grow overflow-auto">
		<div class="contents h-full grow border-r-0">
			<div class="size-full overflow-clip rounded-none transition-all">
				<Obot bind:project bind:currentThreadID {assistant} />
			</div>
		</div>
	</div>
</div>

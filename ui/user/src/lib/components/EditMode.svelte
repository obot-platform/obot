<script lang="ts">
	import { type Project, ChatService, type AssistantTool, type Assistant } from '$lib/services';
	import { onDestroy, onMount } from 'svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Obot from '$lib/components/Obot.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { goto } from '$app/navigation';
	import { responsive } from '$lib/stores';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable(), assistant }: Props = $props();

	const layout = getLayout();
	let projectSaved = '';
	let timer: number = 0;
	let toDelete = $state<Project>();
	const projectTools = getProjectTools();

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

	async function onNewTools(newTools: AssistantTool[]) {
		const response = await ChatService.updateProjectTools(project.assistantID, project.id, {
			items: newTools
		});
		projectTools.tools = response.items;
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
		<div
			class="h-full grow border-r-0"
			class:contents={!layout.projectEditorOpen}
			class:hidden={layout.projectEditorOpen && responsive.isMobile}
		>
			<div
				class="size-full overflow-clip transition-all"
				class:rounded-none={!layout.projectEditorOpen}
			>
				<Obot bind:project bind:currentThreadID {assistant} />
			</div>
		</div>
	</div>
</div>

<Confirm
	msg={`${project.editor ? 'Delete' : 'Remove'} the current agent?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			await ChatService.deleteProject(toDelete.assistantID, toDelete.id);
		} finally {
			const projects = (await ChatService.listProjects()).items;
			if (projects.length > 0) {
				await goto(`/o/${projects[0].id}`);
			} else {
				await goto('/catalog');
			}
			toDelete = undefined;
		}
	}}
	oncancel={() => (toDelete = undefined)}
/>

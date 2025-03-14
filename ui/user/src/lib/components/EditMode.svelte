<script lang="ts">
	import { Trash2 } from 'lucide-svelte';
	import General from '$lib/components/edit/General.svelte';
	import { type Project, ChatService, type AssistantTool, type Assistant } from '$lib/services';
	import { onDestroy, onMount } from 'svelte';
	import Instructions from '$lib/components/edit/Instructions.svelte';
	import Interface from '$lib/components/edit/Interface.svelte';
	import Tools from '$lib/components/edit/Tools.svelte';
	import Knowledge from '$lib/components/edit/Knowledge.svelte';
	import Credentials from '$lib/components/edit/Credentials.svelte';
	import Share from '$lib/components/edit/Share.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { columnResize } from '$lib/actions/resize';
	import Obot from '$lib/components/Obot.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { slide } from 'svelte/transition';
	import Files from '$lib/components/edit/Files.svelte';
	import Tasks from '$lib/components/edit/Tasks.svelte';
	import Profile from './navbar/Profile.svelte';
	import EditorToggle from './navbar/EditorToggle.svelte';
	import Projects from './navbar/Projects.svelte';

	interface Props {
		project: Project;
		tools: AssistantTool[];
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let {
		project = $bindable(),
		tools = $bindable(),
		currentThreadID = $bindable(),
		assistant
	}: Props = $props();

	const layout = getLayout();
	let projectSaved = '';
	let timer: number = 0;
	let nav = $state<HTMLDivElement>();
	let toDelete = $state(false);

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
		tools = (
			await ChatService.updateProjectTools(project.assistantID, project.id, {
				items: newTools
			})
		).items;
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

<div class="flex size-full flex-col bg-surface1">
	{#if layout.projectEditorOpen}
		<!-- Header -->
		<div
			class="relative z-40 flex h-16 w-full items-center justify-between bg-surface1 p-3 shadow-md"
			transition:slide
		>
			<div class="flex items-center gap-2">
				<a href="/home"><img src="/user/images/obot-icon-blue.svg" class="h-8" alt="Obot icon" /></a
				>
				<h1 class="text-xl font-semibold">Obot Editor</h1>
			</div>
			<div class="ml-8 flex grow items-center gap-2">
				<p class="text-sm text-gray">Editing:</p>
				<div class="relative flex grow">
					<Projects
						{project}
						onlyEditable={true}
						classes={{
							button: 'bg-white dark:bg-black hover:bg-surface2 shadow-inner px-4',
							tooltip: 'h-fit default-dialog rounded-b-lg shadow-inner -translate-y-2'
						}}
					/>
				</div>
				<EditorToggle {project} />
			</div>
			<div class="flex items-center">
				<Profile />
			</div>
		</div>
	{/if}

	<div class="flex grow overflow-auto">
		{#if layout.projectEditorOpen}
			<!-- Left Nav -->
			<div
				bind:this={nav}
				class="inset-shadow-sm flex h-full w-1/4 min-w-[320px] flex-col overflow-hidden"
				transition:slide={{ axis: 'x' }}
			>
				<div class="default-scrollbar-thin flex grow flex-col">
					<General bind:project />
					<Instructions bind:project />
					<Tools {tools} {onNewTools} {assistant} />
					<Knowledge {project} />
					<Files {project} />
					<Tasks {project} />
					<Interface bind:project />
					<Credentials {project} {tools} />
					<Share {project} />
					<div class="grow"></div>
				</div>
				<div class="flex justify-end bg-surface1 p-2">
					<button
						class="button flex gap-1 text-gray"
						onclick={() => {
							toDelete = true;
						}}
					>
						<Trash2 class="icon-default" />
						<span>Remove</span>
					</button>
				</div>
			</div>
			<div role="none" class="w-2 translate-x-2 cursor-col-resize" use:columnResize={nav}></div>
		{/if}
		<div
			class="colors-surface3 h-full grow rounded-l-3xl border-r-0 p-2"
			class:contents={!layout.projectEditorOpen}
		>
			<div
				class="size-full overflow-clip rounded-2xl transition-all"
				class:rounded-none={!layout.projectEditorOpen}
			>
				<Obot {project} {tools} bind:currentThreadID />
			</div>
		</div>
	</div>
</div>

<Confirm
	msg="Delete the current Obot?"
	show={toDelete}
	onsuccess={async () => {
		await ChatService.deleteProject(project.assistantID, project.id);
		window.location.href = '/home';
	}}
	oncancel={() => (toDelete = false)}
/>

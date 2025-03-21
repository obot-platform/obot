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
	import { goto } from '$app/navigation';
	import Sites from '$lib/components/edit/Sites.svelte';
	import { responsive } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';

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

	async function copy() {
		const newProject = await ChatService.copyProject(project.assistantID, project.id);
		await goto(`/o/${newProject.id}?edit`);
	}
</script>

<div class="bg-surface1 flex size-full flex-col">
	{#if layout.projectEditorOpen}
		<!-- Header -->
		<div
			class="bg-surface1 relative z-40 flex h-16 w-full items-center justify-between gap-4 p-3 shadow-md md:gap-8"
			transition:slide
		>
			<div class="flex shrink-0 items-center gap-2">
				<a href="/home"><img src="/user/images/obot-icon-blue.svg" class="h-8" alt="Obot icon" /></a
				>
				{#if !responsive.isMobile}
					<h1 class="text-xl font-semibold">Obot Editor</h1>
				{/if}
			</div>
			<div class="flex grow items-center justify-between gap-2">
				<p class="text-gray text-sm">Editing:</p>
				<div class="relative flex max-w-[50vw] grow md:max-w-none">
					<Projects
						{project}
						onlyEditable={true}
						classes={{
							button: 'bg-white dark:bg-black hover:bg-surface2 shadow-inner px-4',
							tooltip: twMerge(
								'h-fit w-screen md:w-full default-dialog shadow-inner -translate-y-1 max-h-[80vh] overflow-y-auto default-scrollbar-thin',
								responsive.isMobile &&
									'rounded-none fixed h-[calc(100vh-64px)] -translate-x-[4px] max-h-[calc(100vh-64px)] translate-y-2'
							)
						}}
					/>
				</div>
			</div>
			<div class="flex items-center">
				<EditorToggle {project} />
				{#if !responsive.isMobile}
					<Profile />
				{/if}
			</div>
		</div>
	{/if}

	<div class="flex grow overflow-auto">
		{#if layout.projectEditorOpen}
			<!-- Left Nav -->
			<div
				bind:this={nav}
				class="flex h-full w-screen flex-col overflow-hidden inset-shadow-xs md:w-1/4 md:min-w-[320px]"
				transition:slide={responsive.isMobile ? { axis: 'y' } : { axis: 'x' }}
			>
				<div
					class="default-scrollbar-thin scrollbar-stable-gutter scrollbar-track-transparent! flex grow flex-col"
				>
					<General bind:project />
					<Instructions bind:project />
					<Tools bind:tools {onNewTools} {assistant} {project} />
					<Knowledge {project} />
					{#if assistant?.websiteKnowledge?.siteTool}
						<Sites {project} />
					{/if}
					<Files {project} />
					<Tasks {project} />
					<Interface bind:project />
					<Credentials {project} {tools} />
					<Share {project} />
					<div class="grow"></div>
				</div>
				<div class="bg-surface1 flex justify-between p-2">
					<button class="button flex items-center gap-1 text-sm" onclick={() => copy()}>
						<span>Copy</span>
					</button>
					<button
						class="button-destructive"
						onclick={() => {
							toDelete = true;
						}}
					>
						<Trash2 class="icon-default" />
						<span>Delete</span>
					</button>
				</div>
			</div>
			{#if !responsive.isMobile}
				<div
					role="none"
					class="w-2 translate-x-2 cursor-col-resize"
					use:columnResize={{ column: nav }}
				></div>
			{/if}
		{/if}
		<div
			class="h-full grow p-2"
			class:contents={!layout.projectEditorOpen}
			class:hidden={layout.projectEditorOpen && responsive.isMobile}
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

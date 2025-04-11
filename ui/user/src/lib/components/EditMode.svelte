<script lang="ts">
	import { GripVertical, Plus, Trash2 } from 'lucide-svelte';
	import General from '$lib/components/edit/General.svelte';
	import {
		type Project,
		ChatService,
		type AssistantTool,
		type Assistant,
		EditorService
	} from '$lib/services';
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
	import { errors, responsive, version } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import Slack from '$lib/components/slack/Slack.svelte';
	import CustomTools from './edit/CustomTools.svelte';
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
	let nav = $state<HTMLDivElement>();
	let toDelete = $state(false);
	let showAdvanced = $state(false);
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

	async function createNew() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}?edit`);
		} catch (error) {
			errors.append((error as Error).message);
		}
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
			<div class="flex grow items-center justify-center gap-2">
				<div class="relative flex max-w-48 grow md:max-w-xs">
					<Projects
						{project}
						onlyEditable={true}
						classes={{
							button: 'bg-white dark:bg-black hover:bg-surface2 shadow-inner px-4',
							tooltip: twMerge(
								'h-fit default-dialog shadow-inner -translate-y-1 max-h-[80vh] overflow-y-auto default-scrollbar-thin',
								responsive.isMobile &&
									'!w-screen !rounded-none !h-[calc(100vh-64px)] !-translate-x-[4px] !max-h-[calc(100vh-64px)] !translate-y-2'
							)
						}}
					/>
				</div>
				<button
					onclick={createNew}
					class={twMerge(
						!responsive.isMobile && 'button-small flex items-center gap-1 text-xs',
						responsive.isMobile && 'icon-button bg-surface3'
					)}
				>
					<Plus class="size-5" />
					{responsive.isMobile ? '' : 'Create New Obot'}
				</button>
			</div>
			<div class="flex items-center">
				<EditorToggle />
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
				<div class="default-scrollbar-thin flex grow flex-col">
					<General bind:project />
					<Instructions bind:project />
					<Tools {onNewTools} />
					<Knowledge {project} />

					{#if showAdvanced}
						<div class="flex flex-col" transition:slide>
							{#if assistant?.websiteKnowledge?.siteTool}
								<Sites {project} />
							{/if}
							{#if version.current.dockerSupported}
								<CustomTools {project} />
							{/if}
							<Files {project} />
							<Tasks {project} bind:currentThreadID />
							<Slack {project} />
							<Interface bind:project />
							<Credentials {project} />
							<Share {project} />
						</div>
					{/if}

					{#if project.editor}
						<div class="mt-auto">
							<button class="button-text" onclick={() => (showAdvanced = !showAdvanced)}>
								<span
									>{showAdvanced
										? 'Collapse Advanced Options...'
										: 'Show Advanced Options...'}</span
								>
							</button>
						</div>
					{/if}
				</div>
				<div class="bg-surface1 flex justify-between p-2">
					<button class="button flex items-center gap-1 text-sm" onclick={() => copy()}>
						<span>Create a Copy</span>
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
					class="border-surface2 dark:border-surface2/50 relative cursor-col-resize border-r bg-transparent pr-0.5 pl-1.5"
					use:columnResize={{ column: nav }}
				>
					<div class="text-on-surface1 absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2">
						<GripVertical class="text-surface3 size-3" />
					</div>
				</div>
			{/if}
		{/if}
		<div
			class="h-full grow border-r-0"
			class:contents={!layout.projectEditorOpen}
			class:hidden={layout.projectEditorOpen && responsive.isMobile}
		>
			<div
				class="size-full overflow-clip transition-all"
				class:rounded-none={!layout.projectEditorOpen}
			>
				<Obot bind:project bind:currentThreadID />
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

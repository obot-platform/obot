<script lang="ts">
	import { Trash2 } from 'lucide-svelte';
	import General from '$lib/components/edit/General.svelte';
	import { type Project, ChatService, type AssistantTool } from '$lib/services';
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
	import Settings from '$lib/components/navbar/Settings.svelte';
	import { X } from 'lucide-svelte/icons';
	import { slide } from 'svelte/transition';
	import ShareDialog from '$lib/components/edit/ShareDialog.svelte';
	import Files from '$lib/components/edit/Files.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		project: Project;
		tools: AssistantTool[];
		currentThreadID?: string;
	}

	let {
		project = $bindable(),
		tools = $bindable(),
		currentThreadID = $bindable()
	}: Props = $props();

	const layout = getLayout();
	let projectSaved = '';
	let timer: number = 0;
	let nav = $state<HTMLDivElement>();
	let toDelete = $state(false);
	let items = $state<EditorItem[]>([]);

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

<div class="colors-surface1 flex size-full flex-col">
	{#if layout.projectEditorOpen}
		<!-- Header -->
		<div class="flex h-16 w-full items-center gap-2 p-5" transition:slide>
			<img src="/user/images/obot-icon-blue.svg" class="h-8" alt="Obot icon" />
			<h1 class="text-xl font-semibold">Obot Editor</h1>
			<div class="grow"></div>
			<button class="icon-button" onclick={() => (layout.projectEditorOpen = false)}>
				<X class="icon-default" />
			</button>
		</div>
	{/if}

	<div class="flex grow overflow-auto">
		{#if layout.projectEditorOpen}
			<!-- Left Nav -->
			<div
				bind:this={nav}
				class="flex h-full w-1/4 min-w-[320px] flex-col overflow-auto pt-5"
				transition:slide={{ axis: 'x' }}
			>
				<General bind:project />
				<Instructions bind:project />
				<Tools {tools} {onNewTools} />
				<Knowledge {project} />
				<Files {project} {items} />
				<Interface bind:project />
				<Credentials {project} {tools} />
				<Share {project} />
				<div class="grow"></div>
				<div class="flex justify-end p-2">
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
			class="colors-surface3 h-full grow rounded-3xl p-2"
			class:contents={!layout.projectEditorOpen}
		>
			<div
				class="size-full overflow-clip rounded-2xl transition-all"
				class:rounded-none={!layout.projectEditorOpen}
			>
				<Obot {project} {tools} bind:currentThreadID />
			</div>
			<div class="absolute bottom-2 left-2 z-30 hidden md:flex">
				<Settings />
				<ShareDialog {project} />
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

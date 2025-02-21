<script lang="ts">
	import { term } from '$lib/stores';
	import Navbar from '$lib/components/Navbar.svelte';
	import Editor from '$lib/components/Editors.svelte';
	import { type AssistantTool, ChatService, type Project, type Version } from '$lib/services';
	import Notifications from '$lib/components/Notifications.svelte';
	import Thread from '$lib/components/Thread.svelte';
	import Threads from '$lib/components/Threads.svelte';
	import { columnResize } from '$lib/actions/resize';
	import { hasTool } from '$lib/tools';
	import { onMount } from 'svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		project: Project;
		items?: EditorItem[];
		tools?: AssistantTool[];
		currentThreadID?: string;
	}

	let {
		project,
		tools = [],
		currentThreadID = $bindable(),
		items = $bindable([])
	}: Props = $props();
	let layout = getLayout();
	let editorVisible = $derived(layout.fileEditorOpen || term.open);
	let version = $state<Version>({});

	let mainInput = $state<HTMLDivElement>();

	onMount(async () => {
		if (tools.length === 0) {
			tools = (await ChatService.listTools(project.assistantID, project.id)).items;
		}
		if (!version) {
			version = await ChatService.getVersion();
		}
	});
</script>

<div class="colors-background flex h-full">
	{#if hasTool(tools ?? [], 'threads')}
		<Threads bind:currentThreadID {project} />
	{/if}

	<div class="flex h-full grow flex-col">
		<div style="height: 76px">
			<Navbar {project} {currentThreadID} tools={tools ?? []} {version} {items} />
		</div>
		<main id="main-content" class="flex" style="height: calc(100% - 76px)">
			<div
				bind:this={mainInput}
				id="main-input"
				class="flex h-full {editorVisible ? 'w-2/5' : 'grow'}"
			>
				<Thread bind:id={currentThreadID} {project} {items} />
			</div>

			{#if editorVisible}
				<div class="w-4 translate-x-4 cursor-col-resize" use:columnResize={mainInput}></div>
				<div
					class="w-3/5 grow rounded-tl-3xl border-4 border-b-0 border-r-0 border-surface2 p-5 transition-all"
				>
					<Editor {project} {currentThreadID} {items} />
				</div>
			{/if}
		</main>

		<Notifications />
	</div>
</div>

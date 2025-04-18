<script lang="ts">
	import { LoaderCircle, Wrench } from 'lucide-svelte/icons';
	import {
		ChatService,
		type AssistantTool,
		type Project,
		type Thread,
		type ToolReference
	} from '$lib/services';
	import ToolCatalog from '../edit/ToolCatalog.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { getToolReferences } from '$lib/context/toolReferences.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Prop {
		project: Project;
		currentThreadID?: string;
		thread?: boolean;
	}

	const projectTools = getProjectTools();
	let { project, currentThreadID = $bindable(), thread }: Prop = $props();
	let catalog = $state<HTMLDialogElement | undefined>();
	let tools = $state<AssistantTool[]>([]);
	let loading = $state(false);

	async function onNewTools(newTools: AssistantTool[]) {
		if (!currentThreadID) {
			return;
		}

		tools = (
			await ChatService.updateProjectThreadTools(project.assistantID, project.id, currentThreadID, {
				items: newTools
			})
		).items;
	}

	$effect(() => {
		if (currentThreadID) {
			fetchThreadTools();
		}
	});

	async function fetchThreadTools() {
		if (!currentThreadID) return;

		// Get tool references from context and thread tools from API
		const toolReferences = getToolReferences();
		const threadTools = await ChatService.listProjectThreadTools(
			project.assistantID,
			project.id,
			currentThreadID
		);

		// Create a map of tool references by ID for faster lookup
		const toolReferenceMap = new Map<string, ToolReference>();
		for (const reference of toolReferences) {
			toolReferenceMap.set(reference.id, reference);
		}

		// Enhance each tool with capability information
		tools = (threadTools?.items || []).map((tool) => ({
			...tool,
			capability: toolReferenceMap.get(tool.id)?.metadata?.category === 'Capability'
		}));
	}

	async function sleep(ms: number): Promise<void> {
		return new Promise((resolve) => setTimeout(resolve, ms));
	}

	async function createThread(): Promise<Thread> {
		let thread = await ChatService.createThread(project.assistantID, project.id);
		while (!thread.ready) {
			await sleep(1000);
			thread = await ChatService.getThread(project.assistantID, project.id, thread.id);
		}
		return thread;
	}

	async function handleClick() {
		catalog?.showModal();
		if (thread && !currentThreadID) {
			loading = true;
			const response = await createThread();
			currentThreadID = response.id;
			fetchThreadTools();
			loading = false;
		}
	}
</script>

<button use:tooltip={'Tools'} class="button-icon-primary" onclick={handleClick}>
	<Wrench class="h-5 w-5" />
</button>

<dialog
	bind:this={catalog}
	use:clickOutside={() => catalog?.close()}
	class="h-full max-h-[100vh] w-full max-w-[100vw] rounded-none md:h-fit md:w-[1200px] md:rounded-xl"
>
	{#if loading}
		<div class="flex h-full w-full grow items-center justify-center gap-2 text-base font-semibold">
			<LoaderCircle class="size-5 animate-spin" /> Loading Tools...
		</div>
	{:else}
		<ToolCatalog
			onSelectTools={onNewTools}
			onSubmit={() => {
				catalog?.close();
			}}
			maxTools={projectTools.maxTools}
			title="Thread Tool Catalog"
			{tools}
			isThreadScoped
		/>
	{/if}
</dialog>

<script lang="ts">
	import { Wrench } from 'lucide-svelte/icons';
	import { ChatService, type AssistantTool, type Project } from '$lib/services';
	import { tools as toolsStore } from '$lib/stores';
	import ToolCatalog from '../edit/ToolCatalog.svelte';

	interface Prop {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID }: Prop = $props();
	let catalog = $state<HTMLDialogElement | undefined>();
	let tools = $state<AssistantTool[]>([]);

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

	async function fetchThreadTools() {
		if (!currentThreadID) {
			return;
		}

		tools = (
			await ChatService.listProjectThreadTools(project.assistantID, project.id, currentThreadID)
		).items;
	}

	$effect(() => {
		if (currentThreadID) {
			fetchThreadTools();
		} else {
			tools = toolsStore.current.tools;
		}
	});
</script>

<button
	class="button-icon-primary"
	onclick={() => {
		fetchThreadTools();
		catalog?.showModal();
	}}
>
	<Wrench class="h-5 w-5" />
</button>

<dialog
	bind:this={catalog}
	class="h-full max-h-[100vh] w-full max-w-[100vw] rounded-none md:h-fit md:w-[1200px] md:rounded-xl"
>
	<ToolCatalog
		onSelectTools={onNewTools}
		onSubmit={() => {
			catalog?.close();
		}}
		maxTools={toolsStore.current.maxTools}
		title="Thread Tools"
		{tools}
	/>
</dialog>

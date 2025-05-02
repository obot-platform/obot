<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import McpTools from '$lib/components/edit/McpTools.svelte';
	import type { Project } from '$lib/services';
	import { Wrench } from 'lucide-svelte';

	interface Prop {
		project: Project;
	}

	let { project }: Prop = $props();
	let dialog = $state<HTMLDialogElement | undefined>();
	let mcpToolsCatalog = $state<ReturnType<typeof McpTools> | undefined>();

	async function handleClick() {
		mcpToolsCatalog?.load();
		dialog?.showModal();
	}
</script>

<button use:tooltip={'Tools'} class="button-icon-primary" onclick={handleClick}>
	<Wrench class="h-5 w-5" />
</button>

<dialog
	bind:this={dialog}
	use:clickOutside={() => {
		dialog?.close();
	}}
	class="h-full max-h-[100vh] w-full max-w-[100vw] rounded-none md:h-fit md:w-2xl md:rounded-xl"
>
	<McpTools {project} bind:this={mcpToolsCatalog} onClose={() => dialog?.close()} />
</dialog>

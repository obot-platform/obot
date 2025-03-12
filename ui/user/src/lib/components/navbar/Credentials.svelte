<script lang="ts">
	import { type AssistantTool, type Project } from '$lib/services';
	import Credentials from '$lib/components/edit/Credentials.svelte';
	import { X } from 'lucide-svelte/icons';

	interface Props {
		project: Project;
		tools: AssistantTool[];
	}

	let { project, tools }: Props = $props();
	let dialog = $state<HTMLDialogElement>();
	let credentials = $state<ReturnType<typeof Credentials>>();

	export async function show() {
		await credentials?.reload();
		dialog?.showModal();
	}
</script>

<dialog
	bind:this={dialog}
	class="h-1/4 min-h-[300px] w-1/3 min-w-[300px] p-5 overflow-visible"
>
	<div class="flex h-full flex-col">
		<button class="absolute right-0 top-0 p-3" onclick={() => dialog?.close()}>
			<X class="icon-default" />
		</button>
		<h1 class="mb-10 text-xl font-semibold">Credentials</h1>
		<Credentials bind:this={credentials} {project} {tools} local />
	</div>
</dialog>

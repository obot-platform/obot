<script lang="ts">
	import { Copy } from 'lucide-svelte';
	import { X } from 'lucide-svelte/icons';
	import { ChatService, type Project } from '$lib/services';
	import { goto } from '$app/navigation';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();

	let dialog = $state<HTMLDialogElement>();

	async function copy() {
		dialog?.close();
		const newProject = await ChatService.copyProject(project.assistantID, project.id);
		await goto(`/o/${newProject.id}`);
	}
</script>

<button class="icon-button" onclick={() => dialog?.showModal()}>
	<Copy class="icon-default" />
</button>

<dialog bind:this={dialog} class="colors-surface1 relative min-w-[400px] max-w-md rounded-3xl p-5">
	<div class="flex flex-col">
		<div class="mb-5 flex items-center gap-2">
			<h1>Copy</h1>
			<Copy class="icon-default" />
		</div>

		<p class="text-sm">Create a personal copy of this Obot that you can customize.</p>

		<div class="mt-5 flex items-center gap-2 self-end">
			<button class="button-secondary" onclick={() => dialog?.close()}> Cancel </button>
			<button class="button-primary" onclick={() => copy()}> Copy </button>
		</div>

		<button class="icon-button absolute right-2 top-2" onclick={() => dialog?.close()}>
			<X class="icon-default" />
		</button>
	</div>
</dialog>

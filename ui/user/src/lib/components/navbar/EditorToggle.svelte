<script lang="ts">
	import { Pencil, X } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { errors } from '$lib/stores';
	import { goto } from '$app/navigation';

	interface Props {
		project: Project;
	}

	const layout = getLayout();

	let { project }: Props = $props();
	let obotEditorDialog = $state<HTMLDialogElement>();

	async function createNew() {
		const assistants = (await ChatService.listAssistants()).items;
		let defaultAssistant = assistants.find((a) => a.default);
		if (!defaultAssistant && assistants.length == 1) {
			defaultAssistant = assistants[0];
		}
		if (!defaultAssistant) {
			errors.append(new Error('failed to find default assistant'));
			return;
		}

		const project = await ChatService.createProject(defaultAssistant.id);
		await goto(`/o/${project.id}?edit`);
	}

	async function copy(project: Project) {
		const newProject = await ChatService.copyProject(project.assistantID, project.id);
		await goto(`/o/${newProject.id}?edit`);
	}
</script>

<button
	onclick={() => {
		if (layout.projectEditorOpen) {
			layout.projectEditorOpen = false;
			return;
		}

		if (project.editor) {
			layout.projectEditorOpen = true;
		} else {
			obotEditorDialog?.showModal();
		}
	}}
	class="group relative mr-1 flex items-center rounded-full bg-transparent p-2 text-xs text-gray transition-[background-color] duration-200 hover:bg-blue hover:px-4 hover:text-white active:bg-blue-700"
>
	{#if layout.projectEditorOpen}
		<X class="h-5 w-5" />
	{:else}
		<Pencil class="h-5 w-5" />
	{/if}
	<span
		class="w-0 overflow-hidden transition-[width] duration-300 group-hover:ml-2 group-hover:w-auto"
	>
		<span
			class="delay-250 inline-block translate-x-full transition-[transform] duration-300 group-hover:translate-x-0"
			>{layout.projectEditorOpen
				? 'Close Editor'
				: project.editor
					? 'Edit Obot'
					: 'Obot Editor'}</span
		>
	</span>
</button>

<dialog bind:this={obotEditorDialog} class="w-full max-w-md p-4">
	<div class="flex flex-col gap-4">
		<button class="icon-button absolute right-4 top-2" onclick={() => obotEditorDialog?.close()}>
			<X class="h-5 w-5" />
		</button>
		<h4 class="w-full border-b border-surface2 p-1 text-lg font-semibold">
			What would you like to do?
		</h4>
		<button class="button" onclick={() => copy(project)}>Copy {project.name}</button>
		<button class="button" onclick={() => createNew()}>Create New Obot</button>
	</div>
</dialog>

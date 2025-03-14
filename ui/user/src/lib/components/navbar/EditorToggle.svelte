<script lang="ts">
	import { Pencil, X } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { ChatService, EditorService, type Project } from '$lib/services';
	import { errors } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { fade } from 'svelte/transition';

	interface Props {
		project: Project;
	}

	const layout = getLayout();

	let { project }: Props = $props();
	let obotEditorDialog = $state<HTMLDialogElement>();

	async function createNew() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}?edit`);
		} catch (error) {
			errors.append((error as Error).message);
		}
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

		obotEditorDialog?.showModal();
	}}
	class="group relative mr-1 flex items-center gap-1 rounded-full bg-blue px-4 py-2 text-xs text-white duration-200 active:bg-blue-600"
	transition:fade
>
	{#if layout.projectEditorOpen}
		<X class="h-5 w-5" />
	{:else}
		<Pencil class="h-5 w-5" />
	{/if}
	<span>{layout.projectEditorOpen ? 'Exit Editor' : 'Obot Editor'}</span>
</button>

<dialog bind:this={obotEditorDialog} class="w-full max-w-md p-4">
	<div class="flex flex-col gap-4">
		<button class="icon-button absolute right-2 top-2" onclick={() => obotEditorDialog?.close()}>
			<X class="h-5 w-5" />
		</button>
		<h4 class="w-full border-b border-surface2 p-1 text-lg font-semibold">
			What would you like to do?
		</h4>
		{#if project.editor}
			<button class="button" onclick={() => (layout.projectEditorOpen = true)}
				>Edit {project.name || 'Untitled'}</button
			>
		{:else}
			<button class="button" onclick={() => copy(project)}>Copy {project.name ?? 'Untitled'}</button
			>
		{/if}
		<button class="button" onclick={() => createNew()}>Create New Obot</button>
	</div>
</dialog>

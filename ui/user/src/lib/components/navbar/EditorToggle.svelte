<script lang="ts">
	import { Pencil, X } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { ChatService, EditorService, type Project } from '$lib/services';
	import { errors, responsive } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { fade, fly, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

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

	let hover = $state(false);
</script>

<button
	onmouseenter={() => (hover = true)}
	onmouseleave={() => (hover = false)}
	onclick={() => {
		if (layout.projectEditorOpen) {
			layout.projectEditorOpen = false;
			return;
		}

		if (project.editor) {
			layout.projectEditorOpen = true;
			return;
		}

		obotEditorDialog?.showModal();
	}}
	class={twMerge(
		'group text-gray relative mr-1 flex items-center rounded-full border p-2 text-xs transition-[background-color] duration-200',
		layout.projectEditorOpen
			? 'border-blue bg-blue text-white md:px-4'
			: 'border-surface3 hover:bg-blue bg-transparent hover:px-4 hover:text-white active:bg-blue-700'
	)}
	transition:fade
>
	{#if layout.projectEditorOpen}
		<X class="h-5 w-5" />
	{:else}
		<Pencil class="h-5 w-5" />
	{/if}
	{#if layout.projectEditorOpen && !responsive.isMobile}
		<span class="ml-1">Exit Editor</span>
	{:else if hover && !responsive.isMobile}
		<span class="flex h-5 items-center" transition:slide={{ axis: 'x' }}>
			<span class="ms-2 inline-block text-nowrap delay-250" transition:fly={{ x: 50 }}>
				Obot Editor
			</span>
		</span>
	{/if}
</button>

<dialog bind:this={obotEditorDialog} class="w-full max-w-md p-4">
	<div class="flex flex-col gap-4">
		<button class="icon-button absolute top-2 right-2" onclick={() => obotEditorDialog?.close()}>
			<X class="h-5 w-5" />
		</button>
		<h4 class="border-surface2 w-full border-b p-1 text-lg font-semibold">
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

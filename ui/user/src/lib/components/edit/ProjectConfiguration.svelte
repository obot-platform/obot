<script lang="ts">
	import { closeAll, getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { LoaderCircle, X } from 'lucide-svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import Memories from '$lib/components/edit/Memories.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { goto } from '$app/navigation';
	import { hasTool } from '$lib/tools';
	import ProjectConfigurationKnowledge from './ProjectConfigurationKnowledge.svelte';
	import Confirm from '../Confirm.svelte';
	import { autoHeight } from '$lib/actions/textarea';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let modifiedProject = $state(project);
	let confirmDelete = $state(false);
	let deleting = $state(false);
	let saving = $state(false);
	const layout = getLayout();

	async function handleDeleteProject() {
		deleting = true;
		// on current project so signal being deleted
		layout.deleting = true;

		await ChatService.deleteProject(project.assistantID, project.id);
		confirmDelete = false;

		const projects = await ChatService.listProjects();
		deleting = false;
		if (projects.items.length > 0) {
			goto(`/o/${projects.items[0].id}`);
		} else {
			goto('/');
		}
	}

	async function handleUpdate() {
		saving = true;
		project = await ChatService.updateProject(modifiedProject);
		saving = false;
		closeAll(layout);
	}

	const projectTools = getProjectTools();
</script>

<div class="min-h-full w-full flex-col bg-gray-50 dark:bg-black">
	<div class="mx-auto min-h-full w-full px-4 py-4 md:max-w-[1200px] md:px-8">
		<div class="mb-4 flex items-center gap-2">
			<h1 class="text-2xl font-semibold capitalize">Project Configuration</h1>
			<div class="flex grow justify-end">
				<button class="icon-button" onclick={() => closeAll(layout)}>
					<X class="size-6" />
				</button>
			</div>
		</div>
		<div class="flex flex-col gap-6">
			<div
				class="dark:bg-surface1 dark:border-surface3 flex h-fit w-full flex-col gap-4 rounded-lg border border-transparent bg-white p-6 shadow-sm"
			>
				<div class="flex gap-6">
					<div class="flex grow flex-col gap-4">
						<div class="flex flex-col gap-1">
							<label class="text-sm" for="name">Name</label>
							<input
								type="text"
								id="name"
								bind:value={modifiedProject.name}
								class="text-input-filled dark:bg-black"
							/>
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm" for="description">Description</label>
							<input
								type="text"
								id="description"
								bind:value={modifiedProject.description}
								class="text-input-filled dark:bg-black"
							/>
						</div>
					</div>
				</div>

				<div class="flex flex-col gap-1">
					<label class="text-sm" for="prompt">Instructions</label>
					<textarea
						rows={6}
						id="prompt"
						bind:value={modifiedProject.prompt}
						class="text-input-filled dark:bg-black"
						placeholder={HELPER_TEXTS.prompt}
						use:autoHeight
					></textarea>
				</div>
			</div>

			<ProjectConfigurationKnowledge project={modifiedProject} />

			{#if hasTool(projectTools.tools, 'memory')}
				<Memories {project} />
			{/if}

			<div class="mb-8 flex flex-col gap-2">
				<h2 class="text-xl font-semibold">Danger Zone</h2>
				<div class="rounded-md border border-red-500 p-4">
					<div class="flex items-center justify-between gap-4">
						<div class="flex flex-col">
							<p class="font-semibold">Delete Project</p>
							<span class="text-sm font-light">Delete this project and all associated data.</span>
						</div>
						<button class="button-destructive text-md" onclick={() => (confirmDelete = true)}>
							Delete this project
						</button>
					</div>
				</div>
			</div>
		</div>
	</div>
	<div
		class="sticky bottom-0 left-0 flex w-full justify-end gap-4 bg-gray-50 p-4 md:px-8 dark:bg-black"
	>
		<button disabled={saving} class="button" onclick={() => closeAll(layout)}> Cancel </button>
		<button disabled={saving} class="button-primary" onclick={handleUpdate}>
			{#if saving}
				<LoaderCircle class="size-4 animate-spin" />
			{:else}
				Update
			{/if}
		</button>
	</div>
</div>

<Confirm
	msg="Are you sure you want to delete this project?"
	show={confirmDelete}
	onsuccess={handleDeleteProject}
	oncancel={() => (confirmDelete = false)}
	loading={deleting}
/>

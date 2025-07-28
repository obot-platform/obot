<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { X } from 'lucide-svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import ProjectConfigurationKnowledge from './ProjectConfigurationKnowledge.svelte';
	import Confirm from '../Confirm.svelte';
	import { goto } from '$app/navigation';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	const layout = getLayout();
	let confirmDelete = $state(false);
	let deleting = $state(false);

	async function handleDeleteProject() {
		deleting = true;
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
</script>

<div class="relative flex h-full w-full justify-center bg-gray-50 dark:bg-black">
	<div class="h-full w-full px-4 py-4 md:max-w-[1200px] md:px-8">
		<div class="mb-4 flex items-center gap-2">
			<h1 class="text-2xl font-semibold capitalize">Project Configuration</h1>
			<div class="flex grow justify-end">
				<button class="icon-button" onclick={() => closeSidebarConfig(layout)}>
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
								bind:value={project.name}
								class="text-input-filled dark:bg-black"
							/>
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm" for="description">Description</label>
							<input
								type="text"
								id="description"
								bind:value={project.description}
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
						bind:value={project.prompt}
						class="text-input-filled dark:bg-black"
						placeholder={HELPER_TEXTS.prompt}
					></textarea>
				</div>
			</div>

			<ProjectConfigurationKnowledge {project} />

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
</div>

<Confirm
	msg="Are you sure you want to delete this project?"
	show={confirmDelete}
	onsuccess={handleDeleteProject}
	oncancel={() => (confirmDelete = false)}
	loading={deleting}
/>

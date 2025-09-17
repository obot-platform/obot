<script lang="ts">
	import { closeAll, getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { LoaderCircle, X, AlertCircle } from 'lucide-svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import Memories from '$lib/components/edit/Memories.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { goto } from '$app/navigation';
	import { hasTool } from '$lib/tools';
	import ProjectConfigurationKnowledge from './ProjectConfigurationKnowledge.svelte';
	import Confirm from '../Confirm.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { poll } from '$lib/utils';
	import { onMount } from 'svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let modifiedProject = $state(project);
	let confirmDelete = $state(false);
	let deleting = $state(false);
	let saving = $state(false);
	let upgradeLoading = $state(false);

	const projectTools = getProjectTools();
	const projectMCPs = getProjectMCPs();
	const layout = getLayout();

	let showUpgradeButton = $derived(
		!!project?.sourceProjectID &&
			project.sourceProjectID.trim() !== '' &&
			project?.templateUpgradeAvailable
	);

	async function upgradeFromTemplate() {
		upgradeLoading = true;
		const lastUpgraded = project.templateLastUpgraded;
		try {
			await ChatService.projectUpgradeFromTemplate(project.assistantID, project.id);
			// Poll until the upgrade completes
			await poll(
				async () => {
					project = await ChatService.getProject(project.id);
					return project?.templateLastUpgraded !== lastUpgraded;
				},
				{ interval: 500, maxTimeout: 30000 }
			);

			// The upgrade was successful
			// Refresh the sidebar mcp servers, tasks, and config form data
			// Knowledge files are refreshed when the project changes because we're using
			// bind:project={modifiedProject} in the ProjectConfigurationKnowledge component below
			modifiedProject = project;
			layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
			projectMCPs.items = await ChatService.listProjectMCPs(project.assistantID, project.id);
		} catch (error) {
			console.error('Failed to upgrade project from template:', error);
		} finally {
			upgradeLoading = false;
		}
	}

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

	onMount(async () => {
		// Get the latest project data so that we know if an upgrade is available
		project = await ChatService.getProject(project.id);
	});
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
			{#if project.sourceProjectID}
				<div class="flex flex-col gap-2">
					<div class="flex items-center gap-2">
						<h2 class="text-xl font-semibold">Shared Project Info</h2>
						<InfoTooltip
							text="This project was created by copying a shared project"
							class="size-4"
							classes={{ icon: 'size-4' }}
						/>
					</div>
					<div
						class="dark:bg-surface1 dark:border-surface3 flex h-fit w-full flex-col gap-4 rounded-lg border border-transparent bg-white p-6 shadow-sm"
					>
						<div class="flex flex-col gap-4">
							{#if project.templatePublicID}
								{@const shareUrl = `${window.location.origin}/t/${project.templatePublicID}`}
								<div class="flex flex-col gap-2">
									<p class="font-semibold">Public Share URL</p>
									<div class="flex flex-wrap items-center gap-2 text-sm">
										<CopyButton text={shareUrl} />
										<span class="line-clamp-1 text-xs break-all text-gray-500">{shareUrl}</span>
									</div>
								</div>
							{/if}

							{#if project.templateLastUpgraded}
								<div class="flex flex-col gap-2">
									<p class="font-semibold">Last Upgraded</p>
									<div class="flex items-center gap-2 text-sm">
										<span>{new Date(project.templateLastUpgraded).toLocaleString()}</span>
										{#if showUpgradeButton}
											<button
												class="button-primary px-2 py-1 text-[10px]"
												onclick={upgradeFromTemplate}
												disabled={upgradeLoading}
											>
												{#if upgradeLoading}
													<LoaderCircle class="size-3 animate-spin" />
												{:else}
													Upgrade
												{/if}
											</button>
										{/if}
									</div>
								</div>
							{/if}

							<p class="mt-1 flex items-start gap-1 text-xs text-gray-500">
								<AlertCircle class="mt-0.5 size-3 text-amber-600 dark:text-amber-400" />
								<span
									>Changing fields such as name, description, instructions, MCP servers, tasks, or
									knowledge will make this project ineligible to receive updates from the shared
									project snapshot author!</span
								>
							</p>
						</div>
					</div>
				</div>
			{/if}

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

			<ProjectConfigurationKnowledge bind:project={modifiedProject} />

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

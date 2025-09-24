<script lang="ts">
	import { closeAll, getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { LoaderCircle, X, AlertCircle, CircleFadingArrowUp } from 'lucide-svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import Memories from '$lib/components/edit/Memories.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { goto } from '$app/navigation';
	import { hasTool } from '$lib/tools';
	import { AlertTriangle } from 'lucide-svelte';
	import ProjectConfigurationKnowledge from './ProjectConfigurationKnowledge.svelte';
	import Confirm from '../Confirm.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { poll } from '$lib/utils';
	import { onMount } from 'svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import PageLoading from '$lib/components/PageLoading.svelte';

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
	const layout = getLayout();

	let copiedProject = $derived(!!project?.sourceProjectID && project.sourceProjectID.trim() !== '');
	let shareUrl = $derived(
		project.templatePublicID ? `${window.location.origin}/t/${project.templatePublicID}` : undefined
	);
	let showUpgradeButton = $derived(
		!!project?.sourceProjectID &&
			project.sourceProjectID.trim() !== '' &&
			project.templateUpgradeAvailable &&
			shareUrl
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
			// Hard refresh the page to ensure all components are updated and auth/config prompts
			// are triggered
			window.location.href = window.location.pathname + '?edit=true';
		} catch (error) {
			console.error('Failed to upgrade project from template:', error);
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
			{#if copiedProject}
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
							<div class="flex flex-col gap-2">
								<div class="flex items-center gap-2">
									<p class="font-semibold">Public Share URL</p>
									{#if !shareUrl}
										<AlertTriangle class="size-4 text-yellow-500" />
									{/if}
								</div>
								{#if shareUrl}
									<div class="text-sm">
										<CopyButton
											showTextLeft={true}
											text={shareUrl}
											buttonText={shareUrl}
											classes={{
												button:
													'button-small flex items-center gap-1 rounded-full border border-gray-500 bg-transparent px-4 py-2 text-gray-600 hover:bg-gray-500 hover:text-white disabled:bg-transparent'
											}}
										/>
									</div>
								{:else}
									<span class="text-sm font-light">Upstream project no longer shared</span>
								{/if}
							</div>

							{#if project.templateLastUpgraded}
								<div class="flex flex-col gap-2">
									<p class="font-semibold">Last Upgraded</p>
									<div class="flex items-baseline gap-2 text-sm">
										<span class="font-light"
											>{new Date(project.templateLastUpgraded).toLocaleString()}</span
										>
										{#if showUpgradeButton}
											<button
												class="button flex gap-1"
												onclick={upgradeFromTemplate}
												disabled={upgradeLoading}
											>
												{#if upgradeLoading}
													<LoaderCircle class="size-4 animate-spin" />
												{:else}
													Upgrade
													<CircleFadingArrowUp class="relative top-[1px] size-4 shrink-0" />
												{/if}
											</button>
										{/if}
									</div>
								</div>
							{/if}

							<p
								class="mt-1 flex w-full items-center justify-center gap-1 text-center text-xs font-light text-gray-600"
							>
								<AlertCircle class="max-h-3.5 min-h-3.5" />
								<span>
									Changing fields such as instructions, MCP servers, tasks, or knowledge will make
									this project ineligible to receive updates from the shared project snapshot
									author.
								</span>
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

<PageLoading show={upgradeLoading} />

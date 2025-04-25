<script lang="ts">
	import type { PageProps } from './$types';
	import Navbar from '$lib/components/Navbar.svelte';
	import { darkMode, errors, responsive } from '$lib/stores';
	import { formatTime } from '$lib/time';
	import { getProjectImage } from '$lib/image';
	import { Origami, Plus, Scroll, Trash2, X } from 'lucide-svelte';
	import {
		ChatService,
		deleteProject,
		EditorService,
		type Project,
		type ProjectShare
	} from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { sortByFeaturedNameOrder } from '$lib/sort';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import FeaturedAgentCard from '$lib/components/agents/FeaturedAgentCard.svelte';
	import AgentCard from '$lib/components/agents/AgentCard.svelte';
	import { goto } from '$app/navigation';

	let { data }: PageProps = $props();

	let agents = $state<Project[]>(
		data.projects.sort((a, b) => {
			return new Date(b.created).getTime() - new Date(a.created).getTime();
		})
	);
	let tools = $state(new Map(data.tools.map((t) => [t.id, t])));

	let toDelete = $state<Project>();
	let createDropdown = $state<HTMLDialogElement>();

	let featured = $derived(data.shares.sort(sortByFeaturedNameOrder));
	let catalogDialog = $state<HTMLDialogElement>();

	async function createNewAgent() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}

	async function makeCopyFromShare(project: ProjectShare) {
		// TEMPORARY HACK TO FAKE COPY
		const response = await ChatService.createProjectFromShare(project.publicID);
		const copy = await ChatService.copyProject(response.assistantID, response.id);
		await ChatService.deleteProject(response.assistantID, response.id);
		await goto(`/o/${copy.id}`);
	}
</script>

<div class="flex min-h-dvh flex-col items-center">
	<Navbar />
	<main
		class="bg-surface1 relative flex w-full grow flex-col items-center justify-center dark:bg-black"
	>
		<div class="flex w-full max-w-(--breakpoint-xl) grow flex-col gap-6 px-4 py-12">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold">Agents</h1>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							createDropdown?.show();
						}}
					>
						<Plus class="size-6" /> Create New Agent
					</button>

					<dialog
						bind:this={createDropdown}
						class="absolute top-12 right-0 left-auto m-0 w-xs"
						use:clickOutside={() => {
							createDropdown?.close();
						}}
					>
						<div class="flex flex-col gap-2 p-2">
							<button
								class="text-md button hover:bg-surface1 dark:hover:bg-surface3 flex w-full items-center gap-2 rounded-sm bg-transparent px-2 font-light"
								onclick={() => {
									catalogDialog?.showModal();
									createDropdown?.close();
								}}
							>
								<Origami class="size-4" /> Create From Template
							</button>
						</div>
						<div class="border-surface2 dark:border-surface3 flex flex-col border-t p-2">
							<button
								class="text-md button hover:bg-surface1 dark:hover:bg-surface3 flex w-full items-center gap-2 rounded-sm bg-transparent px-2 font-light"
								onclick={createNewAgent}
							>
								<Scroll class="size-4" /> Start From Scratch
							</button>
						</div>
					</dialog>
				</div>
			</div>

			<div class="dark:bg-surface2 w-full overflow-hidden rounded-md bg-white shadow-sm">
				<table class="w-full border-collapse">
					<thead class="dark:bg-surface1 bg-surface2">
						<tr>
							<th class="text-md w-1/2 px-4 py-2 text-left font-medium text-gray-500">Agent</th>
							{#if !responsive.isMobile}
								<th class="text-md w-1/4 px-4 py-2 text-left font-medium text-gray-500">Owner</th>
								<th class="text-md w-1/4 px-4 py-2 text-left font-medium text-gray-500">Created</th>
							{/if}
							<th class="text-md float-right w-auto px-4 py-2 text-left font-medium text-gray-500"
								>Actions</th
							>
						</tr>
					</thead>
					<tbody>
						{#each agents as project (project.id)}
							<tr class="border-surface2 dark:border-surface2 border-t shadow-xs">
								<td>
									<a href={`/o/${project.id}`}>
										<div class="flex h-full w-full items-center gap-2 px-4 py-2">
											<div
												class="bg-surface1 flex size-10 flex-shrink-0 items-center rounded-sm p-1 shadow-sm dark:bg-gray-600"
											>
												<img src={getProjectImage(project, darkMode.isDark)} alt={project.name} />
											</div>
											<div class="flex flex-col">
												<h4
													class="line-clamp-1 text-sm font-medium"
													class:text-gray-500={!project.name}
												>
													{project.name || 'Untitled'}
												</h4>
												<p
													class="line-clamp-1 text-xs font-light"
													class:text-gray-300={!project.description}
												>
													{project.description || 'No description'}
												</p>
											</div>
										</div>
									</a>
								</td>
								{#if !responsive.isMobile}
									<td class="text-sm font-light">
										<a class="flex h-full w-full px-4 py-2" href={`/o/${project.id}`}>Unspecified</a
										>
									</td>
									<td class="text-sm font-light">
										<a class="flex h-full w-full px-4 py-2" href={`/o/${project.id}`}
											>{formatTime(project.created)}</a
										>
									</td>
								{/if}
								<td class="flex justify-end px-4 py-2 text-sm font-light">
									<button
										class="icon-button"
										onclick={() => (toDelete = project)}
										use:tooltip={'Delete agent'}
									>
										<Trash2 class="size-4" />
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	</main>
</div>

<Confirm
	msg={`Delete agent: ${toDelete?.name ?? 'Untitled'}?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			await ChatService.deleteProject(toDelete.assistantID, toDelete.id);
			agents = agents.filter((p) => p.id !== toDelete?.id);
		} finally {
			toDelete = undefined;
		}
	}}
	oncancel={() => (toDelete = undefined)}
/>

<dialog
	bind:this={catalogDialog}
	class="w-full p-4"
	use:clickOutside={() => {
		catalogDialog?.close();
	}}
>
	<div class="sticky top-0 right-0 flex w-full justify-end">
		<button
			onclick={() => {}}
			class="text-gray-500 transition-colors duration-300 hover:text-black"
		>
			<X class="size-8" />
		</button>
	</div>
	<div
		class="default-scrollbar-thin flex min-h-0 grow flex-col items-center justify-center gap-4 overflow-y-auto"
	>
		<h2 class="text-3xl font-semibold md:text-4xl">Agent Catalog</h2>
		<p class="mb-4 max-w-full text-center text-base font-light md:max-w-md">
			Check out our featured obots below, or browse all obots to find the perfect one for you. Or if
			you're feeling adventurous, get started and create your own obot!
		</p>
		{#if featured.length > 0}
			<div class="mb-4 flex w-full flex-col items-center justify-center">
				<div class="flex w-full max-w-(--breakpoint-xl) flex-col gap-4 px-4 md:px-12">
					<h3 class="text-2xl font-semibold md:text-3xl">Featured</h3>
					<div class="grid grid-cols-1 gap-x-4 gap-y-6 sm:gap-y-8 lg:grid-cols-2">
						{#each featured.slice(0, 4) as featuredShare}
							<FeaturedAgentCard
								project={featuredShare}
								{tools}
								onclick={() => makeCopyFromShare(featuredShare)}
							/>
						{/each}
					</div>
				</div>
			</div>
		{/if}

		<div class="flex w-full max-w-(--breakpoint-xl) flex-col">
			<div
				class="sticky top-0 z-30 flex items-center gap-4 bg-white px-4 pt-4 pb-2 md:px-12 dark:bg-black"
			>
				<h3 class="text-2xl font-semibold">More Obots</h3>
			</div>
			<div class="grid grid-cols-1 px-4 pt-2 md:grid-cols-2 md:px-12 lg:grid-cols-3">
				{#each data.shares.slice(4) as project}
					<AgentCard {project} {tools} onclick={() => makeCopyFromShare(project)} />
				{/each}
			</div>
		</div>
	</div>
</dialog>

<svelte:head>
	<title>Obot | Agents</title>
</svelte:head>

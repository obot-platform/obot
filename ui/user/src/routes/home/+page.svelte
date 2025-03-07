<script lang="ts">
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import { darkMode } from '$lib/stores';
	import { Copy, Trash2, WrenchIcon } from 'lucide-svelte';
	import { Plus } from 'lucide-svelte/icons';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { ChatService, type ProjectShare } from '$lib/services';
	import { errors } from '$lib/stores';
	import { goto } from '$app/navigation';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { twMerge } from 'tailwind-merge';

	let { data }: PageProps = $props();
	let toDelete = $state<Project>();
	let projects = $state(data.editorProjects);
	let shares = $state<ProjectShare[]>(data.shares.filter((s) => !s.featured));
	let featured = $state<ProjectShare[]>(data.shares.filter((s) => s.featured));
	let tools = $state(new Map(data.tools.map((t) => [t.id, t])));

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
		await goto(`/o/${project.id}`);
	}

	async function copy(project: Project) {
		const newProject = await ChatService.copyProject(project.assistantID, project.id);
		projects.push(newProject);
	}

	function getImage(project: Project | ProjectShare) {
		const imageUrl = darkMode.isDark
			? project.icons?.iconDark || project.icons?.icon
			: project.icons?.icon;

		return imageUrl ?? '/agent/images/placeholder.jpeg'; // need placeholder image
	}
</script>

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-5">
		{#if darkMode.isDark}
			<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
		{:else}
			<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
		{/if}
		<div class="grow"></div>
		<DarkModeToggle />
		<Profile />
	</div>

	{#snippet menu(project: Project)}
		<DotDotDot class="card-icon-button-colors min-h-10 min-w-10 rounded-full p-2.5 text-sm">
			<div class="flex flex-col rounded-xl border-surface2 bg-surface1">
				<button
					class="flex items-center gap-2 rounded-t-xl px-4 py-2 hover:bg-surface3"
					onclick={() => (toDelete = project)}
				>
					<Trash2 class="icon-default" />
					<span>Delete</span>
				</button>
				<button
					class="flex items-center gap-2 rounded-b-xl px-4 py-2 hover:bg-surface3"
					onclick={() => copy(project)}
				>
					<Copy class="icon-default" />
					<span>Copy</span>
				</button>
			</div>
		</DotDotDot>
	{/snippet}

	{#snippet projectCard(project: Project | ProjectShare)}
		<a
			href={'publicID' in project ? `/s/${project.publicID}` : `/o/${project.id}`}
			data-sveltekit-preload-data={'publicID' in project ? 'off' : 'hover'}
			class="card relative z-20 flex-col overflow-hidden shadow-md"
		>
			<div class="absolute left-0 top-0 z-30 flex w-full items-center justify-end p-2">
				<div class="flex items-center justify-end">
					{#if !('publicID' in project)}
						{@render menu(project)}
					{/if}
				</div>
			</div>
			<div class="relative aspect-video">
				<img
					alt="obot logo"
					src={getImage(project)}
					class="absolute left-0 top-0 h-full w-full object-cover opacity-85"
				/>
				<div
					class="absolute -bottom-0 left-0 z-10 h-2/4 w-full bg-gradient-to-b from-transparent via-transparent to-surface1 transition-colors duration-300"
				></div>
			</div>
			<div class="flex h-full flex-col gap-2 px-4 py-2">
				<h4 class="font-semibold">{project.name || 'Untitled'}</h4>
				<p class="line-clamp-3 text-xs text-gray">{project.description}</p>

				{#if 'tools' in project && project.tools}
					<div class="mt-auto flex flex-wrap items-center justify-end gap-2">
						{#each project.tools as tool}
							{@const toolData = tools.get(tool)}
							<div
								class="flex w-fit items-center gap-1 rounded-2xl bg-surface2 p-2 transition-all duration-300"
							>
								{#if toolData?.metadata?.icon}
									<img
										alt={toolData.name || 'Unknown'}
										src={toolData.metadata.icon}
										class={twMerge(
											'h-4 w-4',
											toolData.metadata.icon.endsWith('.svg') && 'dark:invert'
										)}
									/>
								{:else}
									<WrenchIcon class="h-4 w-4" />
								{/if}
							</div>
						{/each}
					</div>
				{:else}
					<div class="min-h-2"></div>
					<!-- placeholder -->
				{/if}
			</div>
		</a>
	{/snippet}

	<main class="colors-background flex w-full max-w-screen-2xl flex-col justify-center px-12 pb-12">
		<div class="mt-8 flex w-full flex-col gap-8">
			{#if featured.length > 0}
				<div class="flex w-full flex-col gap-4">
					<h3 class="text-2xl font-semibold">Featured</h3>
					<div class="featured-card-layout">
						{#each featured as featuredShare}
							{@render projectCard(featuredShare)}
						{/each}
					</div>
				</div>
			{/if}

			<div class="flex w-full flex-col gap-4">
				<h3 class="text-2xl font-semibold">My Obots</h3>
				<div class="card-layout">
					{#each [...projects, ...shares] as project}
						{@render projectCard(project)}
					{/each}
					<button
						class="card flex items-center justify-center gap-1 shadow-md"
						onclick={() => createNew()}
					>
						<Plus class="h-5 w-5" />
						<span class="font-semibold">Create New Obot</span>
					</button>
				</div>
			</div>
		</div>
	</main>

	<Notifications />
</div>

<Confirm
	msg="Delete the Obot {toDelete?.name || 'Untitled'}?"
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			await ChatService.deleteProject(toDelete.assistantID, toDelete.id);
			projects = projects.filter((p) => p.id !== toDelete?.id);
		} finally {
			toDelete = undefined;
		}
	}}
	oncancel={() => (toDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Home</title>
</svelte:head>

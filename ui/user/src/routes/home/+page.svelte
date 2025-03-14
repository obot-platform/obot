<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { Copy, Pencil, Trash2 } from 'lucide-svelte';
	import { Plus } from 'lucide-svelte/icons';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { ChatService, EditorService, type ProjectShare, type ToolReference } from '$lib/services';
	import { errors } from '$lib/stores';
	import { goto } from '$app/navigation';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import ToolPill from '$lib/components/ToolPill.svelte';
	import { getProjectImage } from '$lib/image';

	let { data }: PageProps = $props();
	let toDelete = $state<Project>();
	let featured = $state<ProjectShare[]>(data.shares.filter((s) => s.featured));
	let recentlyUsedProjects = $state<Project[]>(
		data.editorProjects
			.filter((p) => p.editor == false)
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
	);
	let userProjects = $state<Project[]>(
		data.editorProjects
			.filter((p) => p.editor === true)
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
	);
	let tools = $state(new Map(data.tools.map((t) => [t.id, t])));

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

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-5">
		<div class="relative flex items-end">
			{#if darkMode.isDark}
				<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
			{:else}
				<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
			{/if}
			<div class="ml-1.5 -translate-y-1">
				<span
					class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
				>
					BETA
				</span>
			</div>
		</div>
		<div class="grow"></div>
		<div class="flex items-center gap-4">
			<a href="https://docs.obot.ai" rel="external" target="_blank" class="icon-button">Docs</a>
			<a href="https://discord.gg/9sSf4UyAMC" rel="external" target="_blank" class="icon-button">
				{#if darkMode.isDark}
					<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
				{:else}
					<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
				{/if}
			</a>
			<a
				href="https://github.com/obot-platform/obot"
				rel="external"
				target="_blank"
				class="icon-button"
			>
				{#if darkMode.isDark}
					<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-6" />
				{:else}
					<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-6" />
				{/if}
			</a>
			<Profile />
		</div>
	</div>

	{#snippet menu(project: Project)}
		<DotDotDot class="card-icon-button-colors min-h-10 min-w-10 rounded-full p-2.5 text-sm">
			<div class="default-dialog flex flex-col p-2">
				{#if project.editor}
					<button class="menu-button" onclick={() => goto(`/o/${project.id}?edit`)}>
						<Pencil class="icon-default" />
						<span>Edit</span>
					</button>
				{/if}
				<button class="menu-button" onclick={() => copy(project)}>
					<Copy class="icon-default" />
					<span>Copy</span>
				</button>
				{#if project.editor}
					<button class="menu-button" onclick={() => (toDelete = project)}>
						<Trash2 class="icon-default" />
						<span>Delete</span>
					</button>
				{/if}
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
					src={getProjectImage(project, darkMode.isDark)}
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
					<div class="mt-auto flex flex-wrap items-center justify-end gap-2 py-2">
						{#each project.tools.slice(0, 3) as tool}
							{@const toolData = tools.get(tool)}
							{#if toolData}
								<ToolPill tool={toolData} />
							{/if}
						{/each}
						{#if project.tools.length > 3}
							<ToolPill
								tools={project.tools
									.slice(3)
									.map((t) => tools.get(t))
									.filter((t): t is ToolReference => !!t)}
							/>
						{/if}
					</div>
				{:else}
					<div class="min-h-2"></div>
					<!-- placeholder -->
				{/if}
			</div>
		</a>
	{/snippet}

	<main
		class="colors-background relative flex w-full max-w-screen-2xl flex-col justify-center px-4 pb-12 md:px-12"
	>
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

			{#if recentlyUsedProjects.length > 0}
				<div class="flex w-full flex-col gap-4">
					<h3 class="text-2xl font-semibold">Recently Used</h3>
					<div class="card-layout">
						{#each recentlyUsedProjects as project}
							{@render projectCard(project)}
						{/each}
					</div>
				</div>
			{/if}

			<div class="flex w-full flex-col gap-4">
				<div
					class="sticky top-0 z-50 flex items-center gap-4 bg-white py-4 after:absolute after:-left-12 after:-z-10 after:h-[72px] after:w-[100vw] after:bg-white after:content-[''] dark:bg-black after:dark:bg-black"
				>
					<h3 class="text-2xl font-semibold">My Obots</h3>
					<button
						class="button flex items-center gap-1 text-xs font-medium"
						onclick={() => createNew()}
					>
						<Plus class="icon-default" />
						<span>Create New Obot</span>
					</button>
				</div>
				<div class="card-layout">
					{#each userProjects as project}
						{@render projectCard(project)}
					{/each}
					<button
						class="card flex flex-col items-center justify-center whitespace-nowrap p-4 shadow-md md:flex-row"
						onclick={() => createNew()}
					>
						<Plus class="h-8 w-8 md:h-5 md:w-5" />
						<span class="text-sm font-semibold md:text-base">Create New Obot</span>
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
			featured = featured.filter((p) => p.id !== toDelete?.id);
			userProjects = userProjects.filter((p) => p.id !== toDelete?.id);
		} finally {
			toDelete = undefined;
		}
	}}
	oncancel={() => (toDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Home</title>
</svelte:head>

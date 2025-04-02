<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { Copy, Pencil, Trash2 } from 'lucide-svelte';
	import { Plus } from 'lucide-svelte/icons';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { ChatService, EditorService } from '$lib/services';
	import { errors, responsive } from '$lib/stores';
	import { goto } from '$app/navigation';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import ObotCard from '$lib/components/ObotCard.svelte';

	let { data }: PageProps = $props();
	let toDelete = $state<Project>();
	let recentlyUsedProjects = $state<Project[]>(
		data.editorProjects
			.filter((p) => p.editor == false)
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
	);
	let userProjects = $state<Project[]>(
		data.editorProjects.sort(
			(a, b) => new Date(b.created).getTime() - new Date(a.created).getTime()
		)
	);
	let tools = $state(new Map(data.tools.map((t) => [t.id, t])));

	async function createNew() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}

	async function copy(project: Project) {
		const newProject = await ChatService.copyProject(project.assistantID, project.id);
		await goto(`/o/${newProject.id}`);
	}
</script>

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-4 md:p-5">
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
		<div class="flex items-center gap-1">
			{#if !responsive.isMobile}
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
			{/if}
			<Profile />
		</div>
	</div>

	{#snippet actionMenu(project: Project)}
		<DotDotDot class="icon-button min-h-10 min-w-10 p-2.5 text-sm">
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
				<button class="menu-button" onclick={() => (toDelete = project)}>
					<Trash2 class="icon-default" />
					<span>{recentlyUsedProjects.some((p) => p.id === project.id) ? 'Remove' : 'Delete'}</span>
				</button>
			</div>
		</DotDotDot>
	{/snippet}

	<main
		class="colors-background relative flex w-full max-w-(--breakpoint-2xl) flex-col justify-center pb-12"
	>
		<div class="mt-8 flex w-full flex-col gap-8">
			<div class="flex w-full flex-col gap-4">
				<div
					class="sticky top-0 z-30 flex items-center gap-4 bg-white px-4 py-4 md:px-12 dark:bg-black"
				>
					<h3 class="flex flex-shrink-0 text-2xl font-semibold">My Obots</h3>
					<button
						class="button flex items-center gap-1 text-xs font-medium"
						onclick={() => createNew()}
					>
						<Plus class="icon-default" />
						<span>Create New Obot</span>
					</button>
					{#if !responsive.isMobile}
						<div class="flex grow items-center justify-end">
							<a href="/catalog?from=home" class="button-text items-center text-xs underline"
								>View Obot Catalog</a
							>
						</div>
					{/if}
				</div>
				{#if responsive.isMobile}
					<div class="flex grow items-center justify-end">
						<a href="/catalog?from=home" class="button-text items-center py-0 text-sm underline"
							>View Obot Catalog</a
						>
					</div>
				{/if}
				<div class="card-layout px-4 md:px-12">
					{#each userProjects as project}
						<ObotCard {project} {tools}>
							{#snippet menu()}
								{@render actionMenu(project)}
							{/snippet}
						</ObotCard>
					{/each}
					<button
						class="card flex min-h-36 flex-col items-center justify-center p-4 whitespace-nowrap shadow-md"
						onclick={() => createNew()}
					>
						<Plus class="h-8 w-8" />
						<span class="text-sm font-semibold md:text-base">Create New Obot</span>
					</button>
				</div>
			</div>
		</div>
	</main>

	<Notifications />
</div>

<Confirm
	msg={recentlyUsedProjects.some((p) => p.id === toDelete?.id)
		? `Remove recently used Obot ${toDelete?.name || DEFAULT_PROJECT_NAME}?`
		: `Delete the Obot ${toDelete?.name || DEFAULT_PROJECT_NAME}?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			await ChatService.deleteProject(toDelete.assistantID, toDelete.id);
			recentlyUsedProjects = recentlyUsedProjects.filter((p) => p.id !== toDelete?.id);
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

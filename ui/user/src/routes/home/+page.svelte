<script lang="ts">
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import { darkMode } from '$lib/stores';
	import { Copy, ExternalLink, Trash2 } from 'lucide-svelte';
	import { Plus } from 'lucide-svelte/icons';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService } from '$lib/services';
	import { errors } from '$lib/stores';
	import { goto } from '$app/navigation';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { type Project } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';

	let { data }: PageProps = $props();
	let toDelete = $state<Project>();
	let projects = $state(data.editorProjects);

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
		<div class="absolute right-0.5 top-0.5">
			<DotDotDot class="icon-button-colors min-h-10 min-w-10 rounded-full p-2.5 text-sm">
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
		</div>
	{/snippet}

	<main class="colors-background container flex max-w-[1000px] flex-col justify-center p-5">
		<div class="mt-24 flex w-full flex-col gap-5">
			<div class="flex items-center justify-between">
				<h3 class="text-2xl font-semibold">My Obots</h3>
			</div>
			<div class="flex flex-wrap gap-5 rounded-3xl">
				{#each projects as project}
					<a
						href="/o/{project.id}"
						class="button relative flex aspect-video w-48 flex-col items-center justify-center gap-2 rounded-3xl bg-surface1 p-5"
					>
						<div class="flex items-center gap-2">
							<AssistantIcon {project} class="h-8 w-8" />
							<span>{project.name || 'Untitled'}</span>
						</div>
						{#if project.description}
							<p class="text-sm text-gray">This is a description</p>
						{/if}
						{@render menu(project)}
					</a>
				{/each}
				<button
					class="button flex aspect-video w-48 items-center justify-center gap-2 rounded-3xl bg-surface1 p-5"
					onclick={() => createNew()}
				>
					<Plus class="h-5 w-5" />
					<span class="text-lg">New Obot</span>
				</button>
			</div>

			{#if data.shares.length > 0}
				<div class="mt-20 flex items-center justify-between">
					<h3 class="text-2xl font-semibold">Featured</h3>
				</div>
				<div class="flex flex-col gap-2 rounded-3xl">
					{#each data.shares as template}
						<div class="flex w-full items-center gap-2 rounded-3xl bg-surface1 p-10 py-5">
							<AssistantIcon project={template} class="h-8 w-8" />
							<div>
								<span>{template.name || 'Untitled'}</span>
								{#if template.description}
									<p class="text-sm text-gray">This is a description</p>
								{/if}
							</div>
							<div class="grow"></div>
							<a href="/s/{template.publicID}" class="button flex gap-2">
								<ExternalLink class="h-5 w-5" />
								Launch
							</a>
						</div>
					{/each}
				</div>
			{/if}
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

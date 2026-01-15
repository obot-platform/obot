<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import { ChevronDown, Plus, Settings } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import { goto } from '$lib/url';
	import Confirm from '../Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { closeAll, getLayout } from '$lib/context/chatLayout.svelte';
	import PageLoading from '../PageLoading.svelte';
	import { resolve } from '$app/paths';

	interface Props {
		project: Project;
		onOpenChange?: (open: boolean) => void;
		onCreateProject?: () => void;
		disabled?: boolean;
		classes?: {
			button?: string;
			tooltip?: string;
		};
	}

	let {
		project,
		onOpenChange: onProjectOpenChange,
		onCreateProject,
		disabled,
		classes
	}: Props = $props();

	let projects = $state<Project[]>([]);
	let limit = $state(10);
	let open = $state(false);
	let container = $state<HTMLDivElement>();
	let toDelete = $state<Project>();
	let loading = $state(false);
	const layout = getLayout();

	let {
		ref,
		tooltip: buttonPopover,
		toggle
	} = popover({
		placement: 'bottom-start',
		onOpenChange: (value) => {
			open = value;
			onProjectOpenChange?.(value);
		}
	});

	function loadMore() {
		limit += 10;
	}
</script>

<div class="flex" bind:this={container} use:ref>
	<button
		class={twMerge(
			'hover:bg-surface3 bg-primary/10  relative z-10 flex min-h-10 grow items-center justify-between gap-2 truncate py-2 pr-6 pl-2 transition-colors duration-200',
			classes?.button
		)}
		class:hover:bg-surface2={!disabled}
		class:cursor-default={disabled}
		onclick={async () => {
			if (disabled) {
				toggle(false);
				return;
			}
			projects = (await ChatService.listProjects()).items;
			toggle();
		}}
	>
		<div
			class="text-on-background text-md flex w-full max-w-[100%-24px] flex-col truncate text-left"
		>
			<span class="text-[11px] font-normal">Project</span>
			<p class="text-primary text-base font-semibold">{project.name || DEFAULT_PROJECT_NAME}</p>
		</div>
		{#if !disabled}
			<div
				class={twMerge(
					'text-gray translate-x-[1px] transition-transform duration-200',
					open && 'rotate-180'
				)}
			>
				<ChevronDown class="size-5" />
			</div>
		{/if}
	</button>
</div>

{#if open}
	<div
		use:buttonPopover={{ disablePortal: true }}
		class={twMerge(
			'border-surface3 dark:bg-surface1 default-scrollbar-thin bg-background flex max-h-[calc(100vh-123px)] -translate-x-[3px] -translate-y-[3px] flex-col overflow-hidden overflow-y-auto rounded-b-xs border',
			classes?.tooltip
		)}
		style="width: {container?.clientWidth}px"
		role="none"
		onclick={() => toggle(false)}
	>
		{#each projects.slice(0, limit) as p (p.id)}
			{@render ProjectItem(p)}
		{/each}
		{@render LoadMoreButton(projects.length, limit)}

		<button
			class="text-primary hover:bg-primary/10 flex h-14 w-full items-center justify-center gap-1 py-2 text-sm font-medium"
			onclick={() => {
				closeAll(layout);
				onCreateProject?.();
			}}
		>
			<Plus class="size-4" /> Create New Project
		</button>
	</div>
{/if}

{#snippet ProjectItem(p: Project)}
	{@const isActive = p.id === project.id}
	<div
		class={twMerge(
			'group hover:bg-surface3 flex min-h-14 items-center transition-colors',
			isActive && 'bg-surface1 dark:bg-surface2'
		)}
	>
		<a
			href={resolve(`/o/${p.id}`)}
			class="flex min-h-14 w-full items-center gap-2 p-2"
			onclick={() => {
				if (!isActive) loading = true;
			}}
		>
			<div class="flex grow flex-col">
				<span class="text-on-background text-sm font-semibold"
					>{p.name || DEFAULT_PROJECT_NAME}</span
				>
				{#if p.description}
					<span class="text-on-background line-clamp-1 text-xs font-light">{p.description}</span>
				{/if}
			</div>
		</a>
		<button
			class="icon-button hover:text-primary flex-shrink-0 opacity-0 group-hover:opacity-100"
			onclick={() => {
				goto(`/o/${p.id}?edit=true`);
			}}
			use:tooltip={'Configure Project'}
		>
			<Settings class="size-5" />
		</button>
	</div>
{/snippet}

{#snippet LoadMoreButton(totalLength: number, limit: number)}
	{#if totalLength > limit}
		<button
			class="hover:bg-surface2 text-primary mt-1 w-full rounded-sm py-1 text-sm"
			onclick={(e) => {
				e.stopPropagation();
				loadMore();
			}}
		>
			Load 10 more
		</button>
	{/if}
{/snippet}

<Confirm
	msg={toDelete?.editor
		? `Delete the project ${toDelete?.name || DEFAULT_PROJECT_NAME}?`
		: `Remove recently used project ${toDelete?.name || DEFAULT_PROJECT_NAME}?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			await ChatService.deleteProject(toDelete.assistantID, toDelete.id);
		} finally {
			projects = projects.filter((p) => p.id !== toDelete!.id);
			if (toDelete.id === project.id && projects.length > 0) {
				await goto(`/o/${projects[0].id}`);
			} else if (projects.length === 0) {
				await goto('/');
			}
			toDelete = undefined;
		}
	}}
	oncancel={() => (toDelete = undefined)}
/>

<PageLoading show={loading} />

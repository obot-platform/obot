<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { ChevronDown, Settings } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import { goto } from '$app/navigation';
	import Confirm from '../Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { closeAll, getLayout, openConfigureProject } from '$lib/context/chatLayout.svelte';

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
	let buttonElement = $state<HTMLButtonElement>();
	let toDelete = $state<Project>();
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

<div class="flex">
	<div class="h-full w-1 bg-blue-500"></div>
	<button
		bind:this={buttonElement}
		class={twMerge(
			'hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface2 relative z-10 flex min-h-10 grow items-center justify-between gap-2 truncate bg-gray-50 px-3 py-2 transition-colors duration-200',
			classes?.button
		)}
		class:hover:bg-surface2={!disabled}
		class:cursor-default={disabled}
		use:ref
		onclick={async () => {
			if (disabled) {
				toggle(false);
				return;
			}
			projects = (await ChatService.listProjects()).items.sort((a, b) => {
				if (a.id === project.id) return -1;
				if (b.id === project.id) return 1;
				return b.created.localeCompare(a.created);
			});
			toggle();
		}}
	>
		<div
			class="text-on-background text-md flex max-w-[100%-24px] flex-col truncate text-left font-medium"
		>
			<span class="text-[11px] font-normal">Project</span>
			<p>{project.name || DEFAULT_PROJECT_NAME}</p>
		</div>
		{#if !disabled}
			<div class={twMerge('text-gray transition-transform duration-200', open && 'rotate-180')}>
				<ChevronDown class="size-5" />
			</div>
		{/if}
	</button>
</div>

{#if open}
	<div
		use:buttonPopover={{ disablePortal: true }}
		class={twMerge(
			'border-surface3 dark:bg-surface1 flex w-full min-w-xs flex-col overflow-hidden rounded-md border bg-white',
			classes?.tooltip
		)}
		role="none"
		onclick={() => toggle(false)}
	>
		{#each projects.slice(0, limit) as p (p.id)}
			{@render ProjectItem(p)}
		{/each}
		{@render LoadMoreButton(projects.length, limit)}

		<button
			class="hover:bg-surface3 mt-1 h-14 w-full justify-center py-2 text-sm font-medium"
			onclick={() => {
				closeAll(layout);
				onCreateProject?.();
			}}
		>
			Create New Project
		</button>
	</div>
{/if}

{#snippet ProjectItem(p: Project)}
	{@const isActive = p.id === project.id}
	<div
		class={twMerge(
			'group hover:bg-surface3 flex items-center p-2 transition-colors',
			isActive && 'bg-surface1 dark:bg-surface2'
		)}
	>
		<a href="/o/{p.id}" rel="external" class="flex grow items-center gap-2">
			<AssistantIcon project={p} class="shrink-0" />
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
			class="icon-button flex-shrink-0 hover:text-blue-500"
			onclick={() => {
				openConfigureProject(layout, p);
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
			class="hover:bg-surface2 mt-1 w-full rounded-sm py-1 text-sm text-blue-500"
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

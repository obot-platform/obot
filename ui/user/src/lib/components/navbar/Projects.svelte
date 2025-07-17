<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { ChevronDown, Trash2, X } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import { goto } from '$app/navigation';
	import { responsive } from '$lib/stores';
	import Confirm from '../Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		project: Project;
		onOpenChange?: (open: boolean) => void;
		disabled?: boolean;
		classes?: {
			button?: string;
			tooltip?: string;
		};
		showDelete?: boolean;
	}

	let {
		project,
		onOpenChange: onProjectOpenChange,
		disabled,
		classes,
		showDelete
	}: Props = $props();

	let projects = $state<Project[]>([]);
	let limit = $state(10);
	let open = $state(false);
	let buttonElement = $state<HTMLButtonElement>();
	let toDelete = $state<Project>();

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

<button
	bind:this={buttonElement}
	class={twMerge(
		'bg-surface1 dark:border-surface3 relative z-10 flex grow items-center justify-between gap-2 truncate rounded-xl p-2 shadow-inner transition-colors duration-200 dark:border dark:bg-black',
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
	<span class="text-on-background max-w-[100%-24px] truncate text-sm font-semibold">
		{project.name || DEFAULT_PROJECT_NAME}
	</span>
	{#if !disabled}
		<div class={twMerge('text-gray transition-transform duration-200', open && 'rotate-180')}>
			<ChevronDown />
		</div>
	{/if}
</button>

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
	</div>
{/if}

{#snippet ProjectItem(p: Project)}
	{@const isActive = p.id === project.id}
	<div
		class={twMerge(
			'group hover:bg-surface2 dark:hover:bg-surface3 flex items-center p-2 transition-colors',
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
		{#if showDelete}
			<button
				class="flex w-0 flex-shrink-0 items-center justify-center overflow-hidden transition-all duration-300 group-hover:w-6"
				class:w-6={responsive.isMobile}
				onclick={() => (toDelete = p)}
				use:tooltip={{
					disablePortal: true,
					text: p.editor ? 'Delete Project' : 'Remove Project'
				}}
			>
				{#if p.editor}
					<Trash2 class="size-4" />
				{:else}
					<X class="size-4" />
				{/if}
			</button>
		{/if}
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

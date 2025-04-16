<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, EditorService, type Project } from '$lib/services';
	import { ChevronDown, Plus, Trash2, X } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import { goto } from '$app/navigation';
	import { errors, responsive } from '$lib/stores';
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
		onlyEditable?: boolean;
		showCreate?: boolean;
		showDelete?: boolean;
	}

	let {
		project,
		onOpenChange: onProjectOpenChange,
		disabled,
		classes,
		onlyEditable,
		showCreate,
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

	async function createNew() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}
</script>

<button
	bind:this={buttonElement}
	class={twMerge(
		'relative z-10 flex grow items-center justify-between gap-2 truncate rounded-xl p-2',
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
		projects = (await ChatService.listProjects()).items;
		toggle();
	}}
>
	<span class="text-md text-on-background max-w-[100%-24px] truncate font-semibold">
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
		use:buttonPopover
		class={twMerge('flex h-full w-full flex-col', classes?.tooltip)}
		role="none"
		onclick={() => toggle(false)}
		style={onlyEditable ? `width: ${buttonElement?.clientWidth}px` : ''}
	>
		{#each projects.slice(0, limit) as p}
			{@render ProjectItem(p, onlyEditable)}
		{/each}
		{@render LoadMoreButton(projects.length, limit)}
		<a
			href={`/catalog?from=${encodeURIComponent(window.location.pathname)}`}
			class="text-gray hover:bg-surface3 flex items-center justify-center gap-2 px-2 py-4 transition-colors"
		>
			<img src="/user/images/obot-icon-blue.svg" class="h-5" alt="Obot icon" />
			<span class="text-gray text-sm">View Obot Catalog</span>
		</a>
		{#if showCreate}
			<div class="grow"></div>
			<div class="sticky bottom-0 flex justify-center bg-inherit py-3">
				<button
					onclick={createNew}
					class="button-small mx-2 flex w-full items-center justify-center gap-1 py-3 text-sm"
				>
					<Plus class="size-5" /> Create New Obot
				</button>
			</div>
		{/if}
	</div>
{/if}

{#snippet ProjectItem(p: Project, isEditable = false)}
	{@const isActive = p.id === project.id}
	<div
		class={twMerge(
			'group flex items-center rounded-none p-2 transition-colors hover:bg-gray-300 dark:hover:bg-gray-700',
			isActive && 'bg-surface3'
		)}
	>
		<a
			href="/o/{p.id}{isEditable ? '?edit' : ''}"
			rel="external"
			class="flex grow items-center gap-2"
		>
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
					text: p.editor ? 'Delete Obot' : 'Remove Obot'
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
		? `Delete the Obot ${toDelete?.name || DEFAULT_PROJECT_NAME}?`
		: `Remove recently used Obot ${toDelete?.name || DEFAULT_PROJECT_NAME}?`}
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
				await goto('/catalog');
			}
			toDelete = undefined;
		}
	}}
	oncancel={() => (toDelete = undefined)}
/>

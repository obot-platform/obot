<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, EditorService, type Project } from '$lib/services';
	import { Check, ChevronDown, Plus } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import { goto } from '$app/navigation';
	import { errors } from '$lib/stores';

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
	}

	let {
		project,
		onOpenChange: onProjectOpenChange,
		disabled,
		classes,
		onlyEditable,
		showCreate
	}: Props = $props();

	let projects = $state<Project[]>([]);
	let limit = $state(10);
	let open = $state(false);
	let buttonElement = $state<HTMLButtonElement>();

	let { ref, tooltip, toggle } = popover({
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
		use:tooltip
		class={twMerge('flex h-full w-full flex-col p-2', classes?.tooltip)}
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
			class="text-gray hover:bg-surface3 mt-3 flex items-center justify-center gap-2 rounded-full px-2 py-4"
		>
			<img src="/user/images/obot-icon-blue.svg" class="h-5" alt="Obot icon" />
			<span class="text-gray text-sm">View Obot Catalog</span>
		</a>
		{#if showCreate}
			<div class="grow"></div>
			<div class="sticky bottom-0 flex justify-center bg-inherit py-3">
				<button
					onclick={createNew}
					class="button-small flex w-full items-center justify-center gap-1 py-3 text-sm"
				>
					<Plus class="size-5" /> Create New Obot
				</button>
			</div>
		{/if}
	</div>
{/if}

{#snippet ProjectItem(p: Project, isEditable = false)}
	<a
		href="/o/{p.id}{isEditable ? '?edit' : ''}"
		rel="external"
		class="hover:bg-surface3 flex items-center gap-2 rounded-3xl p-2"
	>
		<AssistantIcon project={p} class="shrink-0" />
		<div class="flex grow flex-col">
			<span class="text-on-background text-sm font-semibold">{p.name || DEFAULT_PROJECT_NAME}</span>
			{#if p.description}
				<span class="text-on-background line-clamp-1 text-xs font-light">{p.description}</span>
			{/if}
		</div>
		{#if p.id === project.id}
			<Check class="text-gray mr-2 h-5 w-5 shrink-0" />
		{/if}
	</a>
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

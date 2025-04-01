<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { Check, ChevronDown } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';

	interface Props {
		project: Project;
		onOpenChange?: (open: boolean) => void;
		disabled?: boolean;
		classes?: {
			button?: string;
			tooltip?: string;
		};
		onlyEditable?: boolean;
	}

	let {
		project,
		onOpenChange: onProjectOpenChange,
		disabled,
		classes,
		onlyEditable
	}: Props = $props();

	let projects = $state<Project[]>([]);
	let recentlyUsedLimit = $state(10);
	let myObotsLimit = $state(10);
	let open = $state(false);
	let buttonElement = $state<HTMLButtonElement>();

	let recentlyUsed = $derived(
		projects.length === 0
			? []
			: onlyEditable
				? []
				: projects
						.filter((p) => p.editor === false)
						.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
	);

	let myObots = $derived(
		projects.length === 0
			? []
			: onlyEditable
				? projects
				: projects
						.filter((p) => p.editor === true)
						.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
	);

	let { ref, tooltip, toggle } = popover({
		placement: 'bottom-start',
		onOpenChange: (value) => {
			open = value;
			onProjectOpenChange?.(value);
		}
	});

	function loadMore(category: 'recent' | 'myObots') {
		if (category === 'recent') {
			recentlyUsedLimit += 10;
		} else {
			myObotsLimit += 10;
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
		const results = (await ChatService.listProjects()).items;
		projects = onlyEditable ? results.filter((p) => !!p.editor) : results;
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
		{#if onlyEditable}
			{#each myObots.slice(0, myObotsLimit) as p}
				{@render ProjectItem(p, true)}
			{/each}
			{@render LoadMoreButton(myObots.length, myObotsLimit, 'myObots')}
		{:else}
			{#if recentlyUsed.length > 0}
				<div class="flex flex-col">
					<h3 class="mb-1 px-2 text-sm font-semibold">Recently Used</h3>
					{#each recentlyUsed.slice(0, recentlyUsedLimit) as p}
						{@render ProjectItem(p)}
					{/each}
					{@render LoadMoreButton(recentlyUsed.length, recentlyUsedLimit, 'recent')}
				</div>
			{/if}

			<div class="mt-3 flex flex-col">
				<h3 class="mb-1 px-2 text-sm font-semibold">My Obots</h3>
				{#each myObots.slice(0, myObotsLimit) as p}
					{@render ProjectItem(p)}
				{/each}
				{@render LoadMoreButton(myObots.length, myObotsLimit, 'myObots')}
			</div>

			<a
				href="/home"
				class="text-gray hover:bg-surface3 mt-3 flex items-center justify-center gap-2 rounded-xl px-2 py-4"
			>
				<img src="/user/images/obot-icon-blue.svg" class="h-5" alt="Obot icon" />
				<span class="text-gray text-sm">See All Obots</span>
			</a>
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

{#snippet LoadMoreButton(totalLength: number, limit: number, category: 'recent' | 'myObots')}
	{#if totalLength > limit}
		<button
			class="hover:bg-surface2 mt-1 w-full rounded-sm py-1 text-sm text-blue-500"
			onclick={(e) => {
				e.stopPropagation();
				loadMore(category);
			}}
		>
			Load 10 more
		</button>
	{/if}
{/snippet}

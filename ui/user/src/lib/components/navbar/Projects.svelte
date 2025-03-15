<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, EditorService, type Project } from '$lib/services';
	import { Check, ChevronDown } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { twMerge } from 'tailwind-merge';
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
	}

	let {
		project,
		onOpenChange: onProjectOpenChange,
		disabled,
		classes,
		onlyEditable
	}: Props = $props();
	let projects = $state<Project[]>([]);
	let open = $state(false);

	let { ref, tooltip, toggle } = popover({
		placement: 'bottom-start',
		onOpenChange: (value) => {
			open = value;
			onProjectOpenChange?.(value);
		}
	});

	async function createNew() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}?edit`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}
</script>

<button
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
	<span class="max-w-[100%-24px] truncate text-md font-semibold text-on-background">
		{project.name || 'Untitled'}
	</span>
	{#if !disabled}
		<div class={twMerge('text-gray transition-transform duration-200', open && 'rotate-180')}>
			<ChevronDown />
		</div>
	{/if}
</button>

{#if !disabled}
	<div
		use:tooltip
		class={twMerge('flex h-full w-full flex-col p-2', classes?.tooltip)}
		role="none"
		onclick={() => toggle(false)}
	>
		{#if onlyEditable}
			<button class="button mb-2" onclick={() => createNew()}>Create New Obot</button>
		{/if}
		{#each projects as p}
			<a
				href="/o/{p.id}?sidebar=true{onlyEditable ? '&edit' : ''}"
				rel="external"
				class="flex items-center gap-2 rounded-3xl p-2 hover:bg-surface3"
			>
				<AssistantIcon project={p} class="flex-shrink-0" />
				<div class="flex grow flex-col">
					<span class="text-sm font-semibold text-on-background">{p.name || 'Untitled'}</span>
					{#if p.description}
						<span class="line-clamp-1 text-xs font-light text-on-background">{p.description}</span>
					{/if}
				</div>
				{#if p.id === project.id}
					<Check class="mr-2 h-5 w-5 flex-shrink-0 text-gray" />
				{/if}
			</a>
		{/each}
		{#if !onlyEditable}
			<a
				href="/home"
				class="flex items-center justify-center gap-2 rounded-xl px-2 py-4 text-gray hover:bg-surface3"
			>
				<img src="/user/images/obot-icon-blue.svg" class="h-5" alt="Obot icon" />
				<span class="text-sm text-gray">See All Obots</span>
			</a>
		{/if}
	</div>
{/if}

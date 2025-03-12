<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { Check, ChevronDown } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { getLayout } from '$lib/context/layout.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		project: Project;
		onOpenChange?: (open: boolean) => void;
	}

	let { project, onOpenChange: onProjectOpenChange }: Props = $props();
	let projects = $state<Project[]>([]);
	let layout = getLayout();
	let open = $state(false);

	let { ref, tooltip, toggle } = popover({
		placement: 'bottom-start',
		onOpenChange: (value) => {
			open = value;
			onProjectOpenChange?.(value);
		}
	});
</script>

<button
	class="relative flex grow items-center justify-between gap-2 truncate rounded-xl p-2"
	class:hover:bg-surface2={!layout.projectEditorOpen}
	class:cursor-default={layout.projectEditorOpen}
	use:ref
	onclick={async () => {
		if (layout.projectEditorOpen) {
			toggle(false);
			return;
		}
		projects = (await ChatService.listProjects()).items;
		toggle();
	}}
>
	<span class="max-w-[100% - 24px] text-md truncate font-semibold text-on-background"
		>{project.name || 'Untitled'}</span
	>
	{#if !layout.projectEditorOpen}
		<div class={twMerge('text-gray transition-transform duration-200', open && 'rotate-180')}>
			<ChevronDown />
		</div>
	{/if}
</button>

{#if !layout.projectEditorOpen}
	<div
		use:tooltip
		class="flex h-full w-full -translate-x-14 flex-col border-t-[1px] border-surface3 bg-surface2 p-2 shadow-inner"
		role="none"
		onclick={() => toggle(false)}
	>
		{#each projects as p}
			<a
				href="/o/{p.id}?sidebar=true"
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
		<a
			href="/home"
			class="flex items-center justify-center gap-2 rounded-xl px-2 py-4 text-gray hover:bg-surface3"
		>
			<img src="/user/images/obot-icon-blue.svg" class="h-5" alt="Obot icon" />
			<span class="text-sm text-gray">See All Obots</span>
		</a>
	</div>
{/if}

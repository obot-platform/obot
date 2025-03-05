<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { Check, ChevronDown } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import { getLayout } from '$lib/context/layout.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let projects = $state<Project[]>([]);
	let layout = getLayout();

	let { ref, tooltip, toggle } = popover({
		placement: 'bottom-start'
	});
</script>

<button
	class="flex items-center gap-2 rounded-lg p-2"
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
	<AssistantIcon {project} />
	<span class="text-xl font-semibold text-on-background">{project.name || 'Untitled'}</span>
	{#if !layout.projectEditorOpen}
		<ChevronDown class="text-gray" />
	{/if}
</button>

{#if !layout.projectEditorOpen}
	<div
		use:tooltip
		class="flex min-w-[250px] flex-col rounded-3xl bg-surface1 p-2"
		role="none"
		onclick={() => toggle(false)}
	>
		{#each projects as p}
			<a
				href="/o/{p.id}"
				rel="external"
				class="flex items-center gap-2 rounded-3xl p-2 hover:bg-surface2"
			>
				<AssistantIcon project={p} />
				<div class="flex grow flex-col">
					<span class="text-sm font-medium text-on-background">{p.name || 'Untitled'}</span>
					{#if p.description}
						<p class="text-xs text-gray">{p.description}</p>
					{/if}
				</div>
				{#if p.id === project.id}
					<Check class="h-5 w-5 text-gray" />
				{/if}
			</a>
		{/each}
		<a
			href="/home"
			class="flex items-center justify-center gap-2 rounded-3xl px-2 py-4 text-gray hover:bg-surface2"
		>
			<img src="/user/images/obot-icon-blue.svg" class="h-5" alt="Obot icon" />
			<span class="text-sm font-medium text-gray">Obots</span>
		</a>
	</div>
{/if}

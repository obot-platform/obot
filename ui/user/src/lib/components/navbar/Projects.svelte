<script lang="ts">
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { assistants, context, projects } from '$lib/stores';
	import type { Assistant, Project } from '$lib/services';
	import { Check, ChevronDown, Plus } from 'lucide-svelte/icons';
	import { popover } from '$lib/actions';
	import New from '$lib/components/New.svelte';

	let { ref, tooltip, toggle } = popover({
		placement: 'bottom-start'
	});

	let name = $derived.by(() => {
		return projectName(context.project);
	});

	let newDialog: ReturnType<typeof New>;

	function projectName(p?: Project): string {
		if (p?.name) {
			return p?.name;
		}
		const assistant = getAssistant(p);
		if (assistant?.name) {
			return assistant.name;
		}
		return 'Untitled';
	}

	function getAssistant(p?: Project): Assistant | undefined {
		let assistant = assistants.items.find((a) => a.id === p?.assistantID);
		if (!assistant) {
			assistant = assistants.current();
		}
		return assistant;
	}

	function projectDescription(p?: Project): string {
		if (p?.description) {
			return p?.description;
		}
		const assistant = getAssistant(p);
		if (assistant?.description) {
			return assistant.description;
		}
		return 'Untitled';
	}
</script>

<button
	class="flex items-center gap-2 rounded-lg p-2 hover:bg-surface2"
	use:ref
	onclick={async () => {
		await projects.reload();
		toggle();
	}}
>
	<AssistantIcon />
	<span class="text-xl font-semibold text-on-background">{name}</span>
	<ChevronDown class="text-gray" />
</button>

<div use:tooltip class="flex min-w-[250px] flex-col rounded-3xl bg-surface1 p-2">
	{#each projects.items as project}
		<a
			href="/{getAssistant(project)?.alias || project.assistantID}/projects/{project.id}"
			rel="external"
			class="flex items-center gap-2 rounded-3xl p-2 hover:bg-surface2"
		>
			<AssistantIcon {project} />
			<div class="flex grow flex-col">
				<span class="text-sm font-medium text-on-background">{projectName(project)}</span>
				<p class="text-xs text-gray">{projectDescription(project)}</p>
			</div>
			{#if project.id === context.project?.id}
				<Check class="h-5 w-5 text-gray" />
			{/if}
		</a>
	{/each}
	<button
		class="flex items-center justify-center gap-2 rounded-3xl px-2 py-4 text-gray hover:bg-surface2"
		onclick={async () => {
			await newDialog.show();
			toggle();
		}}
	>
		<Plus class="h-5 w-5 text-gray" />
		<span class="text-sm font-medium text-gray">New Obot</span>
	</button>
</div>

<New bind:this={newDialog} />

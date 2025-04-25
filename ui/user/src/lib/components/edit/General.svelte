<script lang="ts">
	import type { Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { reactiveLabel } from '$lib/actions/reactiveLabel.svelte';
	import EditIcon from './EditIcon.svelte';
	import { Info } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane open>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-semibold">
			General
			{#if !project.editor}
				<div use:tooltip={'Only the owner can modify this section.'}>
					<Info class="size-4 text-gray-500 dark:text-gray-300" />
				</div>
			{/if}
		</span>
	{/snippet}
	<div class="flex flex-col gap-4">
		<div class="flex items-center gap-5">
			<EditIcon bind:project />
		</div>
		<div class="flex flex-col">
			<label for="project-name" use:reactiveLabel={{ value: project.name }}> Name </label>
			<input
				id="project-name"
				disabled={!project.editor}
				type="text"
				placeholder="Name"
				class="bg-surface grow rounded-lg p-2"
				bind:value={project.name}
			/>
		</div>
		<div class="flex flex-col">
			<label for="project-desc" use:reactiveLabel={{ value: project.description }}>
				Description
			</label>
			<textarea
				id="project-desc"
				class="bg-surface grow resize-none rounded-lg p-2"
				disabled={!project.editor}
				rows="1"
				placeholder="Description"
				use:autoHeight
				bind:value={project.description}
			></textarea>
		</div>
	</div>
</CollapsePane>

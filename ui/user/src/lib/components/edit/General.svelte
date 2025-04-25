<script lang="ts">
	import type { Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { reactiveLabel } from '$lib/actions/reactiveLabel.svelte';
	import EditIcon from '$lib/components/edit/EditIcon.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane classes={{ header: 'pl-3 py-2 text-md', content: 'p-2' }} iconSize={5}>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-extralight">
			Name & Description
		</span>
	{/snippet}
	<div class="flex flex-col gap-3 text-sm">
		<div class="mt-2 flex items-center">
			<EditIcon bind:project />
		</div>
		<div class="flex flex-col">
			<label for="project-name" use:reactiveLabel={{ value: project.name }}> Name </label>
			<input
				id="project-name"
				disabled={!project.editor}
				type="text"
				placeholder="Name"
				class="dark:border-surface3 grow rounded-lg bg-white p-2 shadow-sm dark:border dark:bg-black"
				bind:value={project.name}
			/>
		</div>
		<div class="flex flex-col">
			<label for="project-desc" use:reactiveLabel={{ value: project.description }}>
				Description
			</label>
			<textarea
				id="project-desc"
				class="bg-surface dark:border-surface3 grow resize-none rounded-lg p-2 shadow-sm dark:border dark:bg-black"
				disabled={!project.editor}
				rows="1"
				placeholder="Description"
				use:autoHeight
				bind:value={project.description}
			></textarea>
		</div>
	</div>
</CollapsePane>

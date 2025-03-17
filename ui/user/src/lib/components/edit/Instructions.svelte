<script lang="ts">
	import type { Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { reactiveLabel } from '$lib/actions/reactiveLabel.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane header="Instructions">
	<div class="flex flex-col gap-2">
		<div class="flex flex-col gap-2">
			<label for="project-instructions" use:reactiveLabel={{ value: project.prompt }}>
				Instructions
			</label>

			<textarea
				id="project-instructions"
				class="bg-surface grow resize-none rounded-lg p-2"
				rows="3"
				placeholder="Instructions"
				use:autoHeight
				bind:value={project.prompt}
			></textarea>
		</div>
	</div>
</CollapsePane>

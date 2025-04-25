<script lang="ts">
	import type { Project } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea';
	import { reactiveLabel } from '$lib/actions/reactiveLabel.svelte';
	import { popover } from '$lib/actions';
	import { Settings } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { responsive } from '$lib/stores';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let { ref, tooltip: dialog, toggle } = popover();
	const title = project.editor ? 'Instructions' : 'Additional Instructions';
</script>

<div class="flex flex-col gap-2">
	<div class="mb-1 flex items-center justify-between">
		<p class="grow text-sm font-semibold">System Prompt</p>
		<button
			class="icon-button"
			onclick={() => toggle()}
			use:ref
			use:tooltip={'Modify Instructions'}
		>
			<Settings class="size-5" />
		</button>
	</div>
</div>
>

<div
	class="default-dialog bg-surface1 w-xl dark:bg-black"
	use:dialog={{
		slide: responsive.isMobile ? 'up' : undefined,
		fixed: responsive.isMobile ? true : false
	}}
>
	<div class="flex flex-col gap-2 p-4">
		<div class="text-md flex flex-col">
			<p class="text-md mb-4 font-light text-gray-500">
				Describe your agent's personality, goals, and any other relevant information.
			</p>

			<label for="project-instructions" use:reactiveLabel={{ value: project.prompt }}>
				{title}
			</label>

			<textarea
				id="project-instructions"
				class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-4 shadow-sm"
				rows="3"
				placeholder={title}
				use:autoHeight
				bind:value={project.prompt}
			></textarea>
		</div>
	</div>
</div>

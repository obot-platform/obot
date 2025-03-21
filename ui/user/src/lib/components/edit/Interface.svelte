<script lang="ts">
	import type { Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { Plus } from 'lucide-svelte/icons';
	import { Trash2 } from 'lucide-svelte';
	import { reactiveLabel } from '$lib/actions/reactiveLabel.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane header="Interface">
	<div class="flex flex-col gap-4">
		<div class="flex flex-col">
			<label for="project-introduction" use:reactiveLabel={{ value: project.introductionMessage }}>
				Introduction
			</label>
			<textarea
				id="project-introduction"
				class="bg-surface grow resize-none rounded-lg p-2"
				rows="3"
				placeholder="Introduction"
				use:autoHeight
				bind:value={project.introductionMessage}
			></textarea>
		</div>
		{#each project.starterMessages?.keys() ?? [] as i}
			{#if project.starterMessages}
				<div class="flex flex-col gap-2">
					<div class="flex items-center justify-between">
						<label for="project-instructions" class="text-sm">Starter Message {i + 1}</label>
						<button
							onclick={() =>
								(project.starterMessages = [
									...(project.starterMessages ?? []).slice(0, i),
									...(project.starterMessages ?? []).slice(i + 1)
								])}
						>
							<Trash2 class="h-4 w-4" />
						</button>
					</div>
					<textarea
						id="project-instructions"
						class="bg-surface grow resize-none rounded-lg p-2"
						rows="1"
						use:autoHeight
						bind:value={project.starterMessages[i]}
					></textarea>
				</div>
			{/if}
		{/each}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1"
				onclick={() => (project.starterMessages = [...(project.starterMessages ?? []), ''])}
			>
				<Plus class="h-4 w-4" />
				<span class="text-sm">Starter Message</span>
			</button>
		</div>
	</div>
</CollapsePane>

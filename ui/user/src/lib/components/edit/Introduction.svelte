<script lang="ts">
	import type { Project } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea';
	import { Plus } from 'lucide-svelte/icons';
	import { Trash2 } from 'lucide-svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-2' }}
	iconSize={5}
	header="Introduction & Starter Messages"
	helpText={HELPER_TEXTS.introductions}
>
	<div class="flex flex-col gap-4 text-sm">
		<div class="flex w-full flex-col gap-2">
			<label for="project-introduction" class="font-medium">Introduction</label>
			<textarea
				id="project-introduction"
				class="dark:border-surface3 grow resize-none rounded-lg bg-white p-2 shadow-sm dark:border dark:bg-black"
				rows="5"
				placeholder="This will be your agent's go-to message."
				use:autoHeight
				bind:value={project.introductionMessage}
			></textarea>
		</div>

		<div class="flex flex-col gap-2">
			<p class="font-medium">Starter Messages</p>
			<p class="text-xs font-light text-gray-500">
				These messages are conversation options that are provided to the user. Help break the ice
				with your agent by providing a few different options!
			</p>
		</div>

		<div class="flex w-full flex-col gap-4">
			{#each project.starterMessages?.keys() ?? [] as i}
				{#if project.starterMessages}
					<div class="flex gap-2">
						<textarea
							id="project-instructions"
							class="dark:border-surface3 border-surface1 grow resize-none rounded-lg border bg-white p-2 shadow-sm dark:bg-black"
							rows="1"
							use:autoHeight
							bind:value={project.starterMessages[i]}
						></textarea>
						<button
							class="icon-button"
							onclick={() =>
								(project.starterMessages = [
									...(project.starterMessages ?? []).slice(0, i),
									...(project.starterMessages ?? []).slice(i + 1)
								])}
						>
							<Trash2 class="size-4" />
						</button>
					</div>
				{/if}
			{/each}
		</div>
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1"
				onclick={() => (project.starterMessages = [...(project.starterMessages ?? []), ''])}
			>
				<Plus class="size-4" />
				<span class="text-xs">Starter Message</span>
			</button>
		</div>
	</div>
</CollapsePane>

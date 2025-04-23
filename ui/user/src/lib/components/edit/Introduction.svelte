<script lang="ts">
	import type { Project } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea';
	import { Plus, Settings, X } from 'lucide-svelte/icons';
	import EditIcon from './EditIcon.svelte';
	import { Trash2 } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let dialog = $state<HTMLDialogElement | null>(null);
</script>

<div class="flex flex-col gap-2">
	<div class="mb-1 flex items-center justify-between">
		<p class="grow text-sm font-semibold">Introduction & Starter Messages</p>
		<button
			class="icon-button"
			onclick={() => dialog?.showModal()}
			use:tooltip={'Modify Interface'}
		>
			<Settings class="size-5" />
		</button>
	</div>
</div>

<dialog
	bind:this={dialog}
	use:clickOutside={() => dialog?.close()}
	class="bg-surface1 w-3xl overflow-visible p-6 dark:bg-black"
>
	<button
		onclick={() => dialog?.close()}
		class="absolute top-4 right-4 text-gray-500 transition-colors duration-300 hover:text-black"
	>
		<X class="size-8" />
	</button>
	<div class="text-md flex w-full gap-4">
		<EditIcon bind:project inline />
		<div class="flex grow flex-col gap-4 pt-5">
			<div class="flex w-full flex-col gap-1">
				<label for="project-name" class="font-semibold">Name</label>
				<input
					id="project-name"
					disabled={!project.editor}
					type="text"
					class="dark:bg-surface1 grow rounded-lg bg-white p-2 shadow-sm"
					bind:value={project.name}
				/>
			</div>
			<div class="flex w-full flex-col gap-1">
				<label for="project-desc" class="font-semibold">Description</label>
				<textarea
					id="project-desc"
					class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-2 shadow-sm"
					disabled={!project.editor}
					rows="1"
					placeholder="A small blurb or tagline summarizing your agent"
					use:autoHeight
					bind:value={project.description}
				></textarea>
			</div>
			<div class="flex w-full flex-col gap-1">
				<label for="project-introduction" class="font-semibold">Introduction</label>
				<textarea
					id="project-introduction"
					class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-2 shadow-sm"
					rows="5"
					placeholder="This will be your agent's go-to message."
					use:autoHeight
					bind:value={project.introductionMessage}
				></textarea>
			</div>
		</div>
	</div>
	<div class="border-surface-3 mt-8 flex flex-col gap-2 border-t pt-6">
		<h4 class="text-lg font-semibold">Starter Messages</h4>
		<p class="text-sm font-light text-gray-500">
			These messages are conversation options that are provided to the user. <br />
			Help break the ice with your agent by providing a few different options!
		</p>
		<div
			class="default-scrollbar-thin mt-2 flex max-h-36 w-full flex-col gap-2 overflow-y-auto p-1 pr-4"
		>
			{#each project.starterMessages?.keys() ?? [] as i}
				{#if project.starterMessages}
					<div class="flex gap-2">
						<textarea
							id="project-instructions"
							class="dark:bg-surface1 grow resize-none rounded-lg bg-white p-2 shadow-sm"
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
				<span class="text-sm">Starter Message</span>
			</button>
		</div>
	</div>
</dialog>

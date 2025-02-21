<script lang="ts">
	import type { Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { popover } from '$lib/actions';
	import { darkMode } from '$lib/stores';
	import { ChevronDown, CircleX } from 'lucide-svelte/icons';
	import { autoHeight } from '$lib/actions/textarea';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';

	interface Props {
		project: Project;
	}

	$effect(() => {
		if (project.icons?.icon === '' && project.icons?.iconDark === '') {
			project.icons = undefined;
			urlIcon = undefined;
		}
	});
	let { project = $bindable() }: Props = $props();
	let { ref, tooltip, toggle } = popover();
	let urlIcon:
		| {
				icon?: string;
				iconDark?: string;
		  }
		| undefined = $state();
</script>

<CollapsePane header="General" open>
	<div class="flex flex-col gap-2">
		<div class="mb-2 flex items-center gap-5">
			<button class="icon-button flex items-center gap-2" use:ref onclick={() => toggle()}>
				<AssistantIcon {project} class="h-8 w-8" />
				<ChevronDown class="icon-default" />
			</button>
			<div use:tooltip class="z-20 flex flex-col rounded-3xl bg-surface2 p-3">
				{#if urlIcon}
					<div class="flex flex-col gap-2 p-1">
						<div class="flex flex-col gap-2">
							<label for="project-name" class="text-sm">Light Mode URL</label>
							<input
								id="project-name"
								type="text"
								class="bg-surface grow rounded-lg p-2"
								bind:value={urlIcon.icon}
							/>
						</div>
						<div class="flex flex-col gap-2">
							<label for="project-name" class="text-sm">Dark Mode URL (optional)</label>
							<input
								id="project-name"
								type="text"
								class="bg-surface grow rounded-lg p-2"
								bind:value={urlIcon.iconDark}
							/>
						</div>
						<button
							class="button self-end"
							onclick={() => {
								project.icons = urlIcon;
								urlIcon = undefined;
								toggle();
							}}
						>
							Set
						</button>
					</div>
				{:else}
					<div class="grid grid-cols-3">
						{#each [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] as i}
							{@const newLight = `/agent/images/obot_alt_${i}.svg`}
							{@const newDark = `/agent/images/obot_alt_${i}_dark.svg`}
							<button
								class="icon-button"
								onclick={() => {
									project.icons = { icon: newLight, iconDark: newDark };
									toggle();
								}}
							>
								<img class="h-8 w-8" src={darkMode.isDark ? newDark : newLight} alt="Obot icon" />
							</button>
						{/each}
						<button
							class="icon-button flex items-center justify-center"
							onclick={() => {
								project.icons = undefined;
								toggle();
							}}
						>
							<CircleX class="h-5 w-5" />
						</button>
						<button
							class="icon-button col-span-3"
							onclick={() => {
								if (project.icons?.icon && !project.icons.icon.startsWith('/agent/images/')) {
									urlIcon = project.icons;
								} else {
									urlIcon = {};
								}
							}}
						>
							<span class="text-on-surface col-span-2 self-end">Custom URL</span>
						</button>
					</div>
				{/if}
			</div>
		</div>
		<div class="flex flex-col gap-2">
			<label for="project-name" class="text-sm" class:opacity-0={!project.name}>Name</label>
			<input
				id="project-name"
				type="text"
				placeholder="Name"
				class="bg-surface grow rounded-lg p-2"
				bind:value={project.name}
			/>
		</div>
		<div class="flex flex-col gap-2">
			<label for="project-desc" class="text-sm" class:opacity-0={!project.description}
				>Description</label
			>
			<textarea
				id="project-desc"
				class="bg-surface grow resize-none rounded-lg p-2"
				rows="1"
				placeholder="Description"
				use:autoHeight
				bind:value={project.description}
			></textarea>
		</div>
	</div>
</CollapsePane>

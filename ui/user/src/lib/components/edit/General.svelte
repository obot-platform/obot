<script lang="ts">
	import type { Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { popover } from '$lib/actions';
	import { ChevronRight, CircleX, Pencil } from 'lucide-svelte/icons';
	import { autoHeight } from '$lib/actions/textarea';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import GenerateIcon from '$lib/components/edit/GenerateIcon.svelte';
	import UploadIcon from './UploadIcon.svelte';
	import { responsive } from '$lib/stores';
	import { reactiveLabel } from '$lib/actions/reactiveLabel.svelte';

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
	let { ref, tooltip, toggle } = popover({
		slide: responsive.isMobile ? 'left' : undefined,
		fixed: responsive.isMobile ? true : false
	});
	let urlIcon:
		| {
				icon?: string;
				iconDark?: string;
		  }
		| undefined = $state();
</script>

<CollapsePane header="General" open>
	<div class="flex flex-col gap-4">
		<div class="flex items-center gap-5">
			<div class="flex w-full items-center justify-center">
				<button
					class="icon-button group relative flex items-center gap-2 !p-0 shadow-md"
					use:ref
					onclick={() => toggle()}
				>
					<AssistantIcon {project} class="size-24" />
					<div
						class="absolute -right-1 bottom-0 rounded-full bg-surface1 p-2 shadow-md transition-all duration-200 group-hover:bg-surface3"
					>
						<Pencil class="size-4" />
					</div>
				</button>
			</div>
			<div
				use:tooltip
				class="default-dialog left-0 top-16 z-20 flex h-[calc(100vh-64px)] w-screen flex-col px-4 md:left-auto md:top-auto md:h-auto md:w-[350px] md:py-6"
			>
				{#if responsive.isMobile}
					<div class="relative mb-6 flex items-center justify-center border-b border-surface3 py-4">
						<h4 class="text-lg font-medium">Edit Icon</h4>
						<button
							class="icon-button absolute right-0 top-1/2 -translate-y-1/2"
							onclick={() => toggle()}
						>
							<ChevronRight class="size-6" />
						</button>
					</div>
				{/if}
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
					<div class="flex flex-col items-center gap-2">
						<div class="flex justify-center">
							<AssistantIcon {project} class="h-56 w-56" />
						</div>

						<GenerateIcon {project} />

						<div
							class="mt-4 flex w-full flex-col items-center justify-between gap-4 md:flex-row md:gap-0"
						>
							<UploadIcon
								label="Upload Icon"
								onUpload={(imageUrl: string) => {
									project.icons = {
										...project.icons,
										icon: imageUrl,
										iconDark: undefined
									};
								}}
							/>

							<button
								class="icon-button flex items-center justify-center gap-2 py-2"
								onclick={() => {
									project.icons = undefined;
									toggle();
								}}
							>
								<CircleX class="h-5 w-5" />
								<span class="text-sm">Remove icon</span>
							</button>
						</div>
					</div>
				{/if}
			</div>
		</div>
		<div class="flex flex-col gap-1">
			<label for="project-name" use:reactiveLabel={{ value: project.name }}> Name </label>
			<input
				id="project-name"
				type="text"
				placeholder="Name"
				class="bg-surface grow rounded-lg p-2"
				bind:value={project.name}
			/>
		</div>
		<div class="flex flex-col gap-1">
			<label for="project-desc" use:reactiveLabel={{ value: project.description }}>
				Description
			</label>
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

<script lang="ts">
	import { popover } from '$lib/actions';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import type { Project } from '$lib/services';
	import { responsive } from '$lib/stores';
	import GenerateIcon from '$lib/components/edit/GenerateIcon.svelte';
	import UploadIcon from '$lib/components/edit/UploadIcon.svelte';
	import { ChevronRight, CircleX, Pencil } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		project: Project;
		edit?: boolean;
		onSubmit?: () => void;
		inline?: boolean;
		classes?: {
			icon?: string;
		};
	}

	let { project = $bindable(), onSubmit, inline, classes, edit }: Props = $props();

	let urlIcon:
		| {
				icon?: string;
				iconDark?: string;
		  }
		| undefined = $state();

	let { ref, tooltip, toggle } = popover();
	let canEdit = $derived(project.editor || edit);

	$effect(() => {
		if (project.icons?.icon === '' && project.icons?.iconDark === '') {
			project.icons = undefined;
			urlIcon = undefined;
		}
	});
</script>

{#if inline}
	<div class="flex flex-col gap-4">
		{@render content()}
	</div>
{:else}
	<div class="flex w-full items-center justify-center">
		<button
			class="icon-button group relative flex items-center gap-2 p-0 shadow-md"
			class:cursor-default={!canEdit}
			use:ref
			onclick={() => toggle()}
			disabled={!canEdit}
		>
			<AssistantIcon {project} class={twMerge('size-24', classes?.icon)} />

			{#if canEdit}
				<div
					class="bg-surface1 group-hover:bg-surface3 absolute -right-1 bottom-0 rounded-full p-2 shadow-md transition-all duration-200"
				>
					<Pencil class="size-4" />
				</div>
			{/if}
		</button>
	</div>
	<div
		use:tooltip={{
			slide: responsive.isMobile ? 'left' : undefined,
			fixed: responsive.isMobile ? true : false,
			disablePortal: true
		}}
		class="default-dialog bg-surface1 top-16 left-0 z-40 flex h-[calc(100vh-64px)] w-screen flex-col px-4 md:top-auto md:left-auto md:h-auto md:w-[350px] md:py-6 dark:bg-black"
	>
		{@render content()}
	</div>
{/if}

{#snippet content()}
	{#if responsive.isMobile}
		<div class="border-surface3 relative mb-6 flex items-center justify-center border-b py-4">
			<h4 class="text-lg font-medium">Edit Icon</h4>
			<button
				class="icon-button absolute top-1/2 right-0 -translate-y-1/2"
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
				<AssistantIcon {project} class="size-56" />
			</div>

			<GenerateIcon {project} />

			<div class="mt-4 flex w-full flex-col items-center justify-center gap-4 md:flex-row">
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
					class="icon-button flex items-center justify-center gap-2 px-4 py-2"
					onclick={() => {
						project.icons = undefined;
						onSubmit?.();
					}}
				>
					<CircleX class="h-5 w-5" />
					<span class="text-sm">Remove icon</span>
				</button>
			</div>
		</div>
	{/if}
{/snippet}

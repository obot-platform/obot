<script lang="ts">
	import type { ProjectTemplate, MCP } from '$lib/services';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { X } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { responsive } from '$lib/stores';
	import { ChatService } from '$lib/services';
	import { goto } from '$app/navigation';

	interface Props {
		inline?: boolean;
		onBack?: () => void;
		template?: ProjectTemplate;
		mcps?: MCP[];
	}

	let { template, mcps = [], inline = false, onBack }: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();

	export function open(selectedTemplate: ProjectTemplate, templateMcps: MCP[]) {
		template = selectedTemplate;
		mcps = templateMcps;

		dialog?.showModal();
	}

	async function copyAgent() {
		if (!template || !template.publicID) return;
		const project = await ChatService.copyTemplate(template.publicID);
		goto(`/o/${project.id}`);
	}
</script>

{#if inline}
	{@render body()}
{:else}
	<dialog
		bind:this={dialog}
		use:clickOutside={() => dialog?.close()}
		class="default-dialog w-full max-w-xs p-0 sm:max-w-sm md:max-w-md"
		class:mobile-screen-dialog={responsive.isMobile}
	>
		<div class="default-scrollbar-thin w-full overflow-y-auto">
			<button
				class="icon-button absolute top-3 right-3 z-40"
				onclick={() => dialog?.close()}
				use:tooltip={{ disablePortal: true, text: 'Close Project Copy' }}
			>
				<X class="size-6" />
			</button>
			{@render body()}
		</div>
	</dialog>
{/if}

{#snippet body()}
	{#if !template}
		<div class="flex w-full flex-col items-center justify-center gap-4 py-8 text-center">
			<p class="text-lg">Project Template not found or not available.</p>
		</div>
	{:else}
		<div class="flex flex-col p-4 md:p-6">
			<div class="mb-6 flex flex-col items-center text-center">
				<AssistantIcon project={template.projectSnapshot} class="size-24" />
				<h3 class="text-xl font-medium">
					{template.name || template.projectSnapshot.name || 'Unnamed Project'}
				</h3>
				{#if template.created}
					<div class="mt-1 text-xs text-gray-500">
						{new Date(template.created).toLocaleString(undefined, {
							year: 'numeric',
							month: 'short',
							day: 'numeric',
							hour: '2-digit',
							minute: '2-digit'
						})}
					</div>
				{/if}
			</div>

			{#if template.projectSnapshot.description}
				<div class="mb-5 text-center">
					<p class="text-sm text-gray-600 dark:text-gray-300">
						{template.projectSnapshot.description}
					</p>
				</div>
			{/if}

			{#if mcps.length > 0}
				<div class="mb-5 flex flex-col items-center">
					<div class="flex flex-wrap justify-center gap-2">
						{#each mcps as mcp (mcp.id)}
							{@const manifest = mcp.commandManifest ?? mcp.urlManifest}
							{#if manifest}
								<div
									class="flex w-fit items-center gap-1.5 rounded-md bg-gray-50 px-2 py-1 dark:bg-gray-800"
								>
									{#if manifest.icon}
										<div class="flex-shrink-0 rounded-md bg-white p-1 dark:bg-gray-700">
											<img src={manifest.icon} class="size-3.5" alt={manifest.name} />
										</div>
									{/if}
									<span class="truncate text-xs">{manifest.name}</span>
								</div>
							{/if}
						{/each}
					</div>
				</div>
			{/if}

			<div class="mt-2 flex flex-col gap-4 border-t border-gray-100 pt-4 dark:border-gray-700">
				{#if !template.featured}
					<p class="text-center text-xs text-gray-400">
						This project template was published by a third-party user and may include prompts or
						tools not reviewed or verified by our team. It could interact with external systems,
						access additional data sources, or behave in unexpected ways. By continuing, you
						acknowledge that you understand the risks and choose to proceed at your own discretion.
					</p>
				{/if}
				<div class="flex flex-col items-center gap-3">
					{#if onBack}
						<button onclick={onBack} class="button w-full max-w-xs"> Go Back </button>
					{/if}
					<button onclick={copyAgent} class="button-primary w-full max-w-xs">
						{!template.featured ? 'Accept and Copy Project' : 'Copy Project'}
					</button>
				</div>
			</div>
		</div>
	{/if}
{/snippet}

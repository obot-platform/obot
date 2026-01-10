<script lang="ts">
	import type { ProjectTemplate } from '$lib/services';
	import { ChatService } from '$lib/services';
	import { goto } from '$lib/url';

	interface Props {
		onBack?: () => void;
		template?: ProjectTemplate;
	}

	let { template, onBack }: Props = $props();

	async function launchProject() {
		if (!template || !template.publicID) return;
		const project = await ChatService.copyTemplate(template.publicID);
		goto(`/o/${project.id}`);
	}
</script>

{#if !template}
	<div class="flex w-full flex-col items-center justify-center gap-4 py-8 text-center">
		<p class="text-lg">Project Share not found or not available.</p>
	</div>
{:else}
	<div class="flex flex-col">
		<div class="mb-2 flex flex-col items-center text-center">
			<img src="/user/images/share-project-delivery.webp" class="max-h-48" alt="invitation" />
			<h2 class="text-2xl font-semibold">Launch Shared Project</h2>
			<h3 class="text-xl font-medium">
				{template.projectSnapshot.name || 'Unnamed Project'}
			</h3>
			{#if template.projectSnapshotLastUpgraded}
				<div class="text-on-surface1 mt-0.5 text-[12px]">
					Last Updated: {new Date(template.projectSnapshotLastUpgraded).toLocaleString(undefined, {
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

		<div class="mt-2 flex flex-col gap-4 border-t border-gray-100 pt-4 dark:border-gray-700">
			<p class="text-on-surface1 px-2 text-sm font-light">
				This project was shared by a user and may include instructions, MCP servers, knowledge
				files, and task definitions that were not reviewed or verified by our team. It could
				interact with external systems, access additional data sources, or behave in unexpected
				ways. By clicking "Launch Project", you acknowledge that you understand the risks and choose
				to proceed at your own discretion.
			</p>
			<div class="flex flex-col items-center gap-3">
				<button onclick={launchProject} class="button-primary w-full max-w-xs">
					Launch Project
				</button>
				{#if onBack}
					<button onclick={onBack} class="button w-full max-w-xs"> Go Back </button>
				{/if}
			</div>
		</div>
	</div>
{/if}

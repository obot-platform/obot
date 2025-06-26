<script lang="ts">
	import { type MCPServerTool } from '$lib/services';
	import { ChevronDown, ChevronUp } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import { slide } from 'svelte/transition';
	import { responsive } from '$lib/stores';

	interface Props {
		onClose?: () => void;
		onSubmit?: (selected: string[]) => void;
		tools: MCPServerTool[];
	}

	const { tools }: Props = $props();

	let expandedDescriptions = $state<Record<string, boolean>>({});
	let expandedParams = $state<Record<string, boolean>>({});
	let allDescriptionsEnabled = $state(true);
	let allParamsEnabled = $state(false);

	function handleToggleDescription(toolId: string) {
		if (allDescriptionsEnabled) {
			allDescriptionsEnabled = false;
			for (const { id: refToolId } of tools) {
				if (toolId !== refToolId) {
					expandedDescriptions[refToolId] = true;
				}
			}
			expandedDescriptions[toolId] = false;
		} else {
			expandedDescriptions[toolId] = !expandedDescriptions[toolId];
		}

		const expandedDescriptionValues = Object.values(expandedDescriptions);
		if (
			expandedDescriptionValues.length === tools.length &&
			expandedDescriptionValues.every((v) => v)
		) {
			allDescriptionsEnabled = true;
		}
	}
</script>

<div class="flex flex-col gap-2">
	<div class="flex items-center justify-between gap-4">
		<h4 class="text-md font-semibold">Tools</h4>
		<div class="flex flex-wrap justify-end gap-4 border-r border-transparent pr-3">
			<Toggle
				checked={allDescriptionsEnabled}
				onChange={(checked) => {
					allDescriptionsEnabled = checked;
					expandedDescriptions = {};
				}}
				label="Show All Descriptions"
				labelInline
				classes={{
					label: 'text-sm gap-2'
				}}
			/>

			{#if !responsive.isMobile}
				<div class="bg-surface3 h-5 w-0.5"></div>
			{/if}

			<Toggle
				checked={allParamsEnabled}
				onChange={(checked) => {
					allParamsEnabled = checked;
					expandedParams = {};
				}}
				label="Show All Parameters"
				labelInline
				classes={{
					label: 'text-sm gap-2'
				}}
			/>
		</div>
	</div>
	<div class="flex flex-col gap-2 overflow-hidden">
		{#each tools as tool}
			<div
				class="border-surface2 dark:bg-surface2 dark:border-surface3 flex flex-col gap-2 rounded-md border p-3"
			>
				<div class="flex items-center justify-between gap-2">
					<p class="text-md font-semibold">
						{tool.name}
						{#if tool.unsupported}
							<span class="ml-3 text-sm text-gray-500"> ⚠️ Not yet fully supported in Obot </span>
						{/if}
					</p>
					<div class="flex flex-shrink-0 items-center gap-2">
						<button
							class="icon-button h-fit min-h-auto w-fit min-w-auto flex-shrink-0 p-1"
							onclick={() => handleToggleDescription(tool.id)}
						>
							{#if expandedDescriptions[tool.id]}
								<ChevronUp class="size-4" />
							{:else}
								<ChevronDown class="size-4" />
							{/if}
						</button>
					</div>
				</div>
				{#if expandedDescriptions[tool.id] || allDescriptionsEnabled}
					<p in:slide={{ axis: 'y' }} class="text-sm font-light text-gray-500">
						{tool.description}
					</p>
					{#if Object.keys(tool.params ?? {}).length > 0}
						{#if expandedParams[tool.id] || allParamsEnabled}
							<div
								class={'from-surface2 dark:from-surface3 flex w-full flex-shrink-0 bg-linear-to-r to-transparent px-4 py-2 text-xs font-semibold text-gray-500 md:w-sm'}
							>
								Parameters
							</div>
							<div class="flex flex-col px-4 text-xs" in:slide={{ axis: 'y' }}>
								<div class="flex flex-col gap-2">
									{#each Object.keys(tool.params ?? {}) as paramKey}
										<div class="flex flex-col items-center gap-2 md:flex-row">
											<p class="self-start font-semibold text-gray-500 md:min-w-xs">
												{paramKey}
											</p>
											<p class="self-start font-light text-gray-500">
												{tool.params?.[paramKey]}
											</p>
										</div>
									{/each}
								</div>
							</div>
						{/if}
					{/if}
				{/if}
			</div>
		{/each}
	</div>
</div>

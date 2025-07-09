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
	<div class="flex w-full justify-end">
		<div class="flex flex-wrap items-center justify-end gap-2">
			<Toggle
				checked={allDescriptionsEnabled}
				onChange={(checked) => {
					allDescriptionsEnabled = checked;
					expandedDescriptions = {};
				}}
				label="All Descriptions"
				labelInline
				classes={{
					label: 'text-xs gap-2'
				}}
			/>

			{#if !responsive.isMobile}
				<div class="bg-surface3 mx-2 h-5 w-0.5"></div>
			{/if}

			<Toggle
				checked={allParamsEnabled}
				onChange={(checked) => {
					allParamsEnabled = checked;
					expandedParams = {};
				}}
				label="All Parameters"
				labelInline
				classes={{
					label: 'text-xs gap-2'
				}}
			/>
		</div>
	</div>
	<div class="flex flex-col gap-2 overflow-hidden">
		{#each tools as tool}
			<div
				class="border-surface2 dark:bg-surface2 dark:border-surface3 flex flex-col gap-2 rounded-md border"
				class:pb-2={!expandedDescriptions[tool.id] && !allDescriptionsEnabled}
			>
				<div class="flex items-center justify-between gap-2 px-2 pt-2">
					<p class="text-xs font-medium">
						{tool.name}
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
				{#if tool.unsupported}
					<p class="px-2 text-[11px] font-light text-gray-500">
						⚠️ Not yet fully supported in Obot
					</p>
				{/if}
				{#if expandedDescriptions[tool.id] || allDescriptionsEnabled}
					<p
						in:slide={{ axis: 'y' }}
						class="px-2 text-xs font-light text-gray-500"
						class:pb-2={!expandedParams[tool.id] && !allParamsEnabled}
					>
						{tool.description}
					</p>
					{#if Object.keys(tool.params ?? {}).length > 0}
						{#if expandedParams[tool.id] || allParamsEnabled}
							<div
								class={'from-surface2 dark:from-surface3 flex w-full flex-shrink-0 bg-linear-to-r to-transparent p-2 text-xs font-semibold text-gray-500'}
							>
								Parameters
							</div>
							<div class="flex flex-col px-2 pb-2 text-xs" in:slide={{ axis: 'y' }}>
								<div class="flex flex-col gap-2">
									{#each Object.keys(tool.params ?? {}) as paramKey}
										<div class="flex flex-col items-center gap-2 md:flex-row">
											<p class="self-start font-semibold text-gray-500">
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

<script lang="ts">
	import Toggle from '$lib/components/Toggle.svelte';
	import type { ToolOverride } from '$lib/services';
	import {
		conflictIssue,
		effectiveToolName,
		isToolCustomized,
		toolNameIssue
	} from '$lib/services/user/mcp';
	import ToolNameIssueIcon from '../ToolNameIssueIcon.svelte';
	import { TriangleAlert } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		tools: ToolOverride[];
		toolPrefix?: string;
		componentId: string;
		readonly?: boolean;
		lockedTools?: Set<string>;
		lockedReason?: string;
		effectiveNameDuplicates?: Set<string>;
	}

	let {
		tools = $bindable(),
		toolPrefix,
		componentId,
		readonly,
		lockedTools,
		lockedReason,
		effectiveNameDuplicates = new Set()
	}: Props = $props();

	let expandedTools = $state<Record<string, boolean>>({});

	const orderedTools = $derived.by(() => {
		if (!lockedTools?.size) return tools;
		const unlocked: ToolOverride[] = [];
		const locked: ToolOverride[] = [];
		for (const tool of tools) {
			(lockedTools.has(tool.name) ? locked : unlocked).push(tool);
		}
		return locked.length > 0 ? [...unlocked, ...locked] : tools;
	});
</script>

<div class="flex flex-col gap-2">
	{#each orderedTools as tool (tool.name)}
		{@const currentName = (tool.overrideName || '').trim() || tool.name}
		{@const currentDescription = (tool.overrideDescription || '').trim() || tool.description}
		{@const isCustomized = isToolCustomized(tool)}
		{@const name = effectiveToolName(tool.name, tool.overrideName, toolPrefix)}
		{@const conflict =
			tool.enabled !== false ? conflictIssue(name, effectiveNameDuplicates) : undefined}
		{@const toolKey = `${componentId}-${tool.name}`}
		{@const locked = lockedTools?.has(tool.name) ?? false}

		<div
			class={twMerge(
				'dark:bg-base-300 dark:border-base-400 flex items-start gap-2 rounded border border-transparent bg-white p-2 shadow-sm',
				locked && 'opacity-50'
			)}
		>
			<div class="flex min-w-0 grow flex-col gap-2">
				<div class="flex items-start justify-between gap-2">
					<div class="min-w-0 flex-1">
						<div class="flex min-w-0 items-center gap-1.5">
							<div class="min-w-0 flex-1 truncate text-sm font-medium" title={name}>
								{#if toolPrefix}<span class="text-base-content/75">{toolPrefix}</span
									>{/if}{currentName}
							</div>
							{#if tool.enabled !== false}
								<ToolNameIssueIcon issue={conflict ?? toolNameIssue(name)} />
							{/if}
						</div>
						{#if currentDescription}
							<p class="line-clamp-2 text-xs" title={currentDescription}>
								{currentDescription}
							</p>
						{/if}
						{#if locked && lockedReason}
							<p class="text-muted-content mt-1 text-[11px] italic">{lockedReason}</p>
						{/if}
					</div>
					<div class="flex shrink-0 items-center gap-2">
						<Toggle
							checked={tool.enabled === true}
							disabled={readonly || locked}
							onChange={(checked) => {
								tool.enabled = checked;
							}}
							label={tool.enabled ? 'Disable tool' : 'Enable tool'}
							disablePortal
						/>
						<button
							type="button"
							class="btn btn-secondary btn-sm text-xs"
							disabled={readonly || locked}
							onclick={() => {
								expandedTools[toolKey] = !expandedTools[toolKey];
							}}
						>
							{expandedTools[toolKey] ? 'Hide details' : 'Customize'}
						</button>
					</div>
				</div>

				{#if isCustomized}
					<div class="mt-1 flex items-center gap-1 text-[11px] text-amber-600">
						<TriangleAlert class="size-3 shrink-0" />
						<p>
							Modified: This tool has been customized. The description or name has been changed.
						</p>
					</div>
				{/if}

				{#if expandedTools[toolKey]}
					<div class="mt-2 flex flex-col gap-2">
						<label class="flex flex-col gap-1">
							<span class="text-xs text-muted-content">Tool name</span>
							<input
								class="text-input-filled flex-1 text-sm"
								disabled={readonly}
								bind:value={() => tool.overrideName ?? tool.name, (v) => (tool.overrideName = v)}
							/>
						</label>

						<label class="flex flex-col gap-1">
							<span class="text-xs text-muted-content">Description</span>
							<textarea
								class="text-input-filled h-24 resize-none text-xs"
								disabled={readonly}
								bind:value={
									() => tool.overrideDescription ?? tool.description ?? '',
									(v) => (tool.overrideDescription = v)
								}
								placeholder="Enter tool description..."
							></textarea>
						</label>

						<div class="mt-2 flex justify-end">
							<button
								type="button"
								class="btn btn-sm btn-secondary text-xs"
								disabled={readonly}
								onclick={() => {
									tool.overrideName = undefined;
									tool.overrideDescription = undefined;
								}}
							>
								Reset to default
							</button>
						</div>
					</div>
				{/if}
			</div>
		</div>
	{/each}
</div>

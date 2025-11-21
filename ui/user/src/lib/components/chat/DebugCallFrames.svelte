<script lang="ts">
	import type { CallFrame } from '$lib/services';
	import { ChevronDown, ChevronUp, Code, Download, Maximize2, Minimize2 } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import JsonTreeView from '../JsonTreeView.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		calls?: Record<string, CallFrame>;
		runId?: string;
	}

	const { calls, runId }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let treeInfo = $derived(calls ? buildTree(calls) : { tree: {}, rootNodes: [] });
	let expandAll = $state(false);
	let maximized = $state(false);

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
		expandAll = false;
		maximized = false;
	}

	function buildTree(calls: Record<string, CallFrame>) {
		const tree: Record<string, string[]> = {};
		const rootNodes: string[] = [];

		// Sort calls by start timestamp
		const sortedCalls = Object.entries(calls).sort(
			(a, b) => new Date(a[1].start).getTime() - new Date(b[1].start).getTime()
		);

		sortedCalls.forEach(([id, call]) => {
			if (
				call.tool?.modelProvider &&
				(call.tool?.name === 'GPTScript Gateway Provider' || call.tool?.name === 'Obot')
			) {
				return;
			}

			const parentId = call.parentID || '';
			if (!parentId) {
				rootNodes.push(id);
			} else {
				if (!tree[parentId]) {
					tree[parentId] = [];
				}
				tree[parentId].push(id);
			}
		});

		return { tree, rootNodes };
	}

	function handleDownload() {
		const dataStr =
			'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(calls, null, 2));
		const downloadAnchorNode = document.createElement('a');
		downloadAnchorNode.setAttribute('href', dataStr);
		downloadAnchorNode.setAttribute('download', 'calls.json');
		document.body.appendChild(downloadAnchorNode);
		downloadAnchorNode.click();
		downloadAnchorNode.remove();
	}

	function truncateInput(input?: string | object) {
		if (!input) {
			return '';
		}
		const stringified = typeof input === 'string' ? input : JSON.stringify(input);
		return stringified?.length > 100 ? stringified.slice(0, 100) + '...' : stringified;
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class={twMerge(
		'bg-surface1 dark:bg-surface2 h-full px-0 pb-0 transition-all',
		maximized ? 'h-dvh max-h-dvh w-full max-w-dvw' : 'max-h-[75vh] w-full max-w-2xl'
	)}
	classes={{ title: 'w-full justify-between', header: 'px-4' }}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2">
			<Code class="size-6" />
			<p class="text-xl font-semibold">Run ID: {runId}</p>
		</div>
		<button
			class="button-icon"
			onclick={() => (maximized = !maximized)}
			use:tooltip={maximized ? 'Minimize' : 'Maximize'}
		>
			{#if maximized}
				<Minimize2 class="size-4" />
			{:else}
				<Maximize2 class="size-4" />
			{/if}
		</button>
	{/snippet}
	<div class="bg-background flex max-h-[calc(100%-4rem)] grow flex-col rounded-md">
		<div class="flex items-center justify-between gap-4 px-4 pt-2 pb-4">
			<h4 class="text-lg font-semibold">Call Frames</h4>
			<div class="flex items-center gap-2">
				<button
					class="button-icon"
					onclick={handleDownload}
					use:tooltip={{
						disablePortal: true,
						text: 'Download JSON'
					}}
				>
					<Download class="size-5" />
				</button>
				<button
					class="button-icon"
					onclick={() => (expandAll = !expandAll)}
					use:tooltip={{
						disablePortal: true,
						text: expandAll ? 'Collapse All' : 'Expand All'
					}}
				>
					{#if expandAll}
						<ChevronUp class="size-5" />
					{:else}
						<ChevronDown class="size-5" />
					{/if}
				</button>
			</div>
		</div>

		<div
			class="default-scrollbar-thin text-md flex min-h-0 grow flex-col overflow-y-auto px-4 pb-4"
		>
			{#each treeInfo.rootNodes as nodeId (nodeId)}
				{@render nodeContent(nodeId)}
			{/each}
		</div>
	</div>
</ResponsiveDialog>

{#snippet nodeContent(nodeId: string, depth = 0)}
	{@const call = calls?.[nodeId]}
	{@const children = treeInfo.tree[nodeId]}
	<details open={expandAll}>
		<summary class="cursor-pointer">
			{#if call}
				{@render summaryContent(call)}
			{/if}
		</summary>
		<div class="my-2 ml-5">
			{#if call?.tool?.source?.location && call.tool.source.location !== 'inline'}
				<div class="mb-2 text-sm text-gray-400 dark:text-gray-600">
					Source:{' '}
					<a
						href={call.tool.source.location}
						target="_blank"
						rel="noopener noreferrer"
						class="text-link"
					>
						{call.tool.source.location}
					</a>
				</div>
			{/if}
			<details open={expandAll}>
				<summary class="cursor-pointer">
					Input Message: {truncateInput(call?.input)}
				</summary>
				{#if call?.input}
					<div class="ml-5">
						{@render inputContent(call.input)}
					</div>
				{:else}
					<p class="ml-5 text-gray-400 dark:text-gray-600">No input available</p>
				{/if}
			</details>
			<details open={expandAll}>
				<summary class="cursor-pointer">Output Messages</summary>
				<ul class="ml-5 list-none">
					{#if call?.output && call.output.length > 0}
						{@const flatOutput = call.output.flat()}
						{#each flatOutput as output, index (index)}
							{#if output.content}
								<li class="my-2">
									<details open={expandAll}>
										<summary class="cursor-pointer">
											{truncateInput(output.content)}
										</summary>
										<p class="ml-5 py-2 whitespace-pre-wrap">
											{output.content}
										</p>
									</details>
								</li>
							{:else if output.subCalls}
								{#each Object.entries(output.subCalls) as [subCallKey, subCall]}
									<li class="mb-2">
										<details open={expandAll}>
											<summary class="cursor-pointer">
												Tool call: {truncateInput(subCallKey)}
											</summary>
											<p class="ml-5 whitespace-pre-wrap">
												Tool Call ID: {subCallKey}
											</p>
											<p class="ml-5 whitespace-pre-wrap">
												Tool ID: {subCall.toolID}
											</p>
											<p class="ml-5 whitespace-pre-wrap">
												Input: {subCall.input}
											</p>
										</details>
									</li>
								{/each}
							{:else}
								<li>
									<p class="text-gray-400 dark:text-gray-600">No output available</p>
								</li>
							{/if}
						{/each}
					{/if}
				</ul>
			</details>
			{#if children && children.length > 0}
				<details open={expandAll}>
					<summary class="cursor-pointer">Subcalls</summary>
					<div class="ml-5">
						{#each children as childId (childId)}
							{@render nodeContent(childId, depth + 1)}
						{/each}
					</div>
				</details>
			{/if}
			{#if call?.llmRequest || call?.llmResponse}
				<details open={expandAll}>
					<summary class="cursor-pointer">
						{call.llmRequest?.chatCompletion?.messages
							? 'LLM Request & Response'
							: 'Tool Command and Output'}
					</summary>
					<div class="ml-5">
						{#if call?.llmRequest}
							<details open={expandAll}>
								<summary class="cursor-pointer">
									{call.llmRequest?.chatCompletion?.messages ? 'Request' : 'Command'}
								</summary>
								<div class="ml-5">{@render inputContent(call.llmRequest)}</div>
							</details>
						{/if}
						{#if call?.llmResponse}
							<details open={expandAll}>
								<summary class="cursor-pointer">
									{call.llmRequest?.chatCompletion?.messages ? 'Response' : 'Output'}
								</summary>
								<div class="ml-5">{@render inputContent(call.llmResponse)}</div>
							</details>
						{/if}
					</div>
				</details>
			{/if}
			{#if call?.tool?.toolMapping}
				<details open={expandAll}>
					<summary class="cursor-pointer">Tools</summary>
					<div class="ml-5">
						{@render toolMappingContent(call.tool.toolMapping)}
					</div>
				</details>
			{/if}
			{#if call?.tool?.export}
				<details open={expandAll}>
					<summary class="cursor-pointer">Shared Tools</summary>
					<div class="ml-5">{@render exportsContent(call.tool.export)}</div>
				</details>
			{/if}
		</div>
	</details>
{/snippet}

{#snippet summaryContent(call: CallFrame)}
	{@const name =
		call.tool?.name || call.tool?.source?.repo || call.tool?.source?.location || 'main'}
	{@const startTime = new Date(call.start).toLocaleTimeString()}
	{@const endTime = call.end ? new Date(call.end).toLocaleTimeString() : 'In progress'}
	{@const duration = call.end
		? `${((new Date(call.end).getTime() - new Date(call.start).getTime()) / 1000).toFixed(2)}s`
		: 'N/A'}
	{@const category = call.tool?.type || 'tool'}
	{@const info = `[${category || 'tool'}] [ID: ${call.id}] [${startTime} - ${endTime}, ${duration}]`}

	<p class="inline">
		<span class="mr-2 text-base font-semibold">
			{typeof name === 'string' ? name : name.Name}
		</span>
		<span class="text-sm font-light text-gray-400 dark:text-gray-600">{info}</span>
	</p>
{/snippet}

{#snippet inputContent(input?: string | object)}
	{#if input}
		{@const parsedInput = (() => {
			try {
				return typeof input === 'string' ? JSON.parse(input) : input;
			} catch {
				return null;
			}
		})()}
		{#if parsedInput}
			<JsonTreeView data={parsedInput} expanded={expandAll} />
		{:else}
			<p class="ml-5 py-2 whitespace-pre-wrap">{input}</p>
		{/if}
	{/if}
{/snippet}

{#snippet toolMappingContent(toolMapping: Record<string, any>)}
	{#each Object.entries(toolMapping) as [key, value] (key)}
		<div class="mb-2">
			{#if value.some((item: any) => item.toolID !== key)}
				{key}:
				<ul class="ml-5 list-none">
					{#each value as item (item.toolID)}
						<li class="mb-2">
							<p class="ml-5 whitespace-pre-wrap">{item.reference}</p>
							<p class="ml-5 whitespace-pre-wrap">{item.toolID}</p>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="whitespace-pre-wrap">{key}</p>
			{/if}
		</div>
	{/each}
{/snippet}

{#snippet exportsContent(exports: string[])}
	<ul class="ml-5 list-none">
		{#each exports as item, index (index)}
			<li class="mb-2">
				<p class="whitespace-pre-wrap">{item}</p>
			</li>
		{/each}
	</ul>
{/snippet}

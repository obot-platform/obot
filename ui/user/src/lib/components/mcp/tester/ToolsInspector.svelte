<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import { m } from '$lib/i18n';
	import type { DirectOperationResult, MCPTesterSession } from '$lib/services/mcp/tester.svelte';
	import CapabilityList, { type CapabilityListItem } from './CapabilityList.svelte';
	import JsonSchemaForm from './JsonSchemaForm.svelte';
	import McpResult from './McpResult.svelte';
	import { Ban, Play } from '@lucide/svelte';

	interface Props {
		session: MCPTesterSession;
	}

	let { session }: Props = $props();
	let inspector = $derived(session.inspectors.tools);
	let cache = $derived(session.cache.tools);
	let selected = $derived(cache.items.find((tool) => tool.name === inspector.selectedName));
	let listItems = $derived<CapabilityListItem[]>(
		cache.items.map((tool) => ({
			id: tool.name,
			title: tool.title || tool.name,
			description: tool.description,
			metadata: tool.name
		}))
	);
	let callActive = $derived(session.activeWorkflow?.label === 'tool call');

	function selectTool(name: string) {
		inspector.selectedName = name;
		inspector.argumentsValue = undefined;
		inspector.result = undefined;
	}

	async function callTool() {
		if (!selected || !inspector.argumentsValue) return;
		const name = selected.name;
		const args = inspector.argumentsValue;
		inspector.result = undefined;
		const result = await session.callTool(name, args);
		if (inspector.selectedName !== name) return;
		inspector.result = result;
	}

	$effect(() => {
		if (!cache.loaded && !cache.loading && !cache.error && !session.activeWorkflow) {
			void session.loadTools();
		}
	});
</script>

<div class="flex h-full min-h-0 flex-col">
	<h2 class="sr-only">{m.mcps_tester_tools()}</h2>

	{#if cache.unsupported}
		<div class="bg-base-200 dark:bg-base-300 shrink-0 rounded-lg p-5" role="status">
			<h3 class="font-medium">{m.mcps_tester_not_supported()}</h3>
			<p class="mt-1 text-sm text-muted-content">
				{m.mcps_tester_tools_unsupported()}
			</p>
		</div>
	{:else if cache.error && !cache.loading}
		<div class="notification-error mb-4 shrink-0 p-4" role="alert">
			<strong
				>{cache.errorStatus === 'cancelled'
					? m.mcps_tester_loading_cancelled()
					: m.mcps_tester_tools_load_failed()}</strong
			>
			<p class="mt-1 text-sm">{cache.error}</p>
			<button class="btn btn-secondary btn-sm mt-3" onclick={() => session.loadTools(true)}
				>{m.mcps_retry()}</button
			>
		</div>
	{/if}

	{#if !cache.unsupported}
		<div
			class="default-scrollbar-thin grid min-h-0 flex-1 gap-6 overflow-y-auto md:grid-cols-[minmax(15rem,0.8fr)_minmax(0,1.7fr)] md:overflow-hidden"
		>
			<CapabilityList
				label={m.mcps_tester_tools()}
				items={listItems}
				selectedId={inspector.selectedName}
				loading={cache.loading}
				busy={Boolean(session.activeWorkflow)}
				onselect={selectTool}
				onrefresh={() => session.loadTools(true)}
				oncancel={() => session.cancelActiveWorkflow()}
			/>

			<section
				class="default-scrollbar-thin min-w-0 md:min-h-0 md:overflow-y-auto md:pr-1"
				aria-label={m.mcps_tester_tool_details()}
			>
				{#if selected}
					<div class="space-y-5">
						<div>
							<h3 class="text-lg font-semibold break-all">{selected.title || selected.name}</h3>
							{#if selected.title}<p class="text-xs text-muted-content break-all">
									{selected.name}
								</p>{/if}
							{#if selected.description}<p class="mt-2 text-sm">{selected.description}</p>{/if}
						</div>

						{#key JSON.stringify(selected.inputSchema)}
							<JsonSchemaForm
								schema={selected.inputSchema}
								disabled={Boolean(session.activeWorkflow)}
								onvalidchange={(value) => {
									inspector.argumentsValue = value;
									inspector.result = undefined;
								}}
							/>
						{/key}

						{#if selected.outputSchema}
							<details>
								<summary class="cursor-pointer text-sm font-medium"
									>{m.mcps_tester_output_schema()}</summary
								>
								<JsonPreview
									value={selected.outputSchema}
									class="mt-2"
									ariaLabel={m.mcps_tester_tool_output_schema()}
								/>
							</details>
						{/if}
						<details>
							<summary class="cursor-pointer text-sm font-medium"
								>{m.mcps_tester_raw_tool_metadata()}</summary
							>
							<JsonPreview
								value={selected}
								class="mt-2"
								ariaLabel={m.mcps_tester_raw_tool_metadata()}
							/>
						</details>

						<div class="flex flex-wrap gap-2">
							<button
								type="button"
								class="btn btn-primary"
								disabled={!inspector.argumentsValue || Boolean(session.activeWorkflow)}
								onclick={callTool}
							>
								<Play class="size-4" aria-hidden="true" />
								{m.mcps_tester_call()}
							</button>
							{#if callActive}
								<button class="btn btn-secondary" onclick={() => session.cancelActiveWorkflow()}>
									<Ban class="size-4" aria-hidden="true" />
									{m.common_cancel()}
								</button>
							{/if}
						</div>

						{#if inspector.result}
							<McpResult
								result={inspector.result as DirectOperationResult<unknown>}
								content={inspector.result.value?.content ?? []}
								structuredContent={inspector.result.value?.structuredContent}
							/>
						{/if}
					</div>
				{:else}
					<div
						class="bg-base-200 dark:bg-base-300 rounded-lg p-6 text-center text-sm text-muted-content"
					>
						{m.mcps_tester_select_tool()}
					</div>
				{/if}
			</section>
		</div>
	{/if}
</div>

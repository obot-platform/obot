<script lang="ts">
	import {
		isTextualMimeType,
		type DirectOperationResult,
		type MCPTesterSession
	} from '$lib/services/mcp/tester.svelte';
	import CapabilityList, { type CapabilityListItem } from './CapabilityList.svelte';
	import McpContent from './McpContent.svelte';
	import McpResult from './McpResult.svelte';
	import { Ban, MessageSquarePlus } from '@lucide/svelte';
	import type { ResourceContents } from '@modelcontextprotocol/sdk/types.js';

	interface Props {
		session: MCPTesterSession;
		onstaged: () => void;
	}

	let { session, onstaged }: Props = $props();
	let inspector = $derived(session.inspectors.resources);
	let cache = $derived(session.cache.resources);
	let selected = $derived(cache.items.find((resource) => resource.uri === inspector.selectedURI));
	let listItems = $derived<CapabilityListItem[]>(
		cache.items.map((resource) => ({
			id: resource.uri,
			title: resource.title || resource.name || resource.uri,
			description: resource.description,
			metadata: [resource.mimeType, resource.uri].filter(Boolean).join(' · ')
		}))
	);
	let readActive = $derived(session.activeWorkflow?.label === 'resource read');
	let resourceStageable = $derived(
		(inspector.result?.value?.contents.length ?? 0) > 0 &&
			(inspector.result?.value?.contents.every(
				(content) => 'text' in content && isTextualMimeType(content.mimeType)
			) ??
				false)
	);

	function renderedContent(content: ResourceContents): unknown {
		if ('text' in content) return { type: 'text', text: content.text };
		if ('blob' in content && content.mimeType?.startsWith('image/')) {
			return { type: 'image', data: content.blob, mimeType: content.mimeType };
		}
		if ('blob' in content && content.mimeType?.startsWith('audio/')) {
			return { type: 'audio', data: content.blob, mimeType: content.mimeType };
		}
		return content;
	}

	async function selectResource(uri: string) {
		inspector.selectedURI = uri;
		inspector.result = undefined;
		inspector.stageError = undefined;
		if (session.activeWorkflow) return;
		await readResource(uri);
	}

	async function retryRead() {
		if (!inspector.selectedURI || session.activeWorkflow) return;
		inspector.result = undefined;
		inspector.stageError = undefined;
		await readResource(inspector.selectedURI);
	}

	async function readResource(uri: string) {
		const result = await session.readResource(uri);
		if (inspector.selectedURI !== uri) return;
		inspector.result = result;
	}

	function useInChat() {
		if (!selected || !inspector.result?.value) return;
		const staged = session.stageResource(
			selected.title || selected.name || selected.uri,
			inspector.result.value
		);
		if (!staged.ok) {
			inspector.stageError = staged.message;
			return;
		}
		onstaged();
	}

	$effect(() => {
		if (!cache.loaded && !cache.loading && !cache.error && !session.activeWorkflow) {
			void session.loadResources();
		}
	});
</script>

<div class="flex h-full min-h-0 flex-col">
	<h2 class="sr-only">Resources</h2>

	{#if cache.unsupported}
		<div class="bg-base-200 dark:bg-base-300 shrink-0 rounded-lg p-5" role="status">
			<h3 class="font-medium">Not supported</h3>
			<p class="mt-1 text-sm text-muted-content">
				This server does not advertise resource support.
			</p>
		</div>
	{:else if cache.error && !cache.loading}
		<div class="notification-error mb-4 shrink-0 p-4" role="alert">
			<strong
				>{cache.errorStatus === 'cancelled'
					? 'Loading cancelled'
					: 'Resources could not be loaded'}</strong
			>
			<p class="mt-1 text-sm">{cache.error}</p>
			<button class="btn btn-secondary btn-sm mt-3" onclick={() => session.loadResources(true)}
				>Retry</button
			>
		</div>
	{/if}

	{#if !cache.unsupported}
		<div
			class="default-scrollbar-thin grid min-h-0 flex-1 gap-6 overflow-y-auto md:grid-cols-[minmax(15rem,0.8fr)_minmax(0,1.7fr)] md:overflow-hidden"
		>
			<CapabilityList
				label="Resources"
				items={listItems}
				selectedId={inspector.selectedURI}
				loading={cache.loading}
				busy={Boolean(session.activeWorkflow)}
				onselect={selectResource}
				onrefresh={() => session.loadResources(true)}
				oncancel={() => session.cancelActiveWorkflow()}
			/>

			<section
				class="default-scrollbar-thin min-w-0 md:min-h-0 md:overflow-y-auto md:pr-1"
				aria-label="Resource details"
			>
				{#if selected}
					<div class="space-y-5">
						<div>
							<h3 class="text-lg font-semibold break-all">
								{selected.title || selected.name || selected.uri}
							</h3>
							<p class="mt-1 text-xs text-muted-content break-all">{selected.uri}</p>
							{#if selected.mimeType}<span class="badge badge-outline mt-2"
									>{selected.mimeType}</span
								>{/if}
							{#if selected.description}<p class="mt-2 text-sm">{selected.description}</p>{/if}
						</div>

						{#if readActive}
							<div class="flex items-center gap-3" aria-live="polite">
								<span class="loading loading-spinner loading-sm"></span>
								<span class="text-sm">Reading resource…</span>
								<button
									class="btn btn-secondary btn-sm"
									onclick={() => session.cancelActiveWorkflow()}
								>
									<Ban class="size-4" aria-hidden="true" /> Cancel
								</button>
							</div>
						{/if}

						{#if inspector.result?.value}
							<div class="space-y-3" aria-label="Resource contents">
								{#each inspector.result.value.contents as content, index (index)}
									<div class="space-y-2">
										<p class="text-xs font-medium break-all">{content.uri}</p>
										<McpContent content={renderedContent(content)} />
									</div>
								{/each}
								{#if !resourceStageable}
									<p class="text-sm text-muted-content">
										Binary or unsupported content remains inspectable but cannot be staged for Chat.
									</p>
								{/if}
							</div>
						{/if}

						{#if inspector.result}
							<McpResult result={inspector.result as DirectOperationResult<unknown>} />
							{#if inspector.result.status !== 'success' && inspector.result.status !== 'cancelled'}
								<button class="btn btn-secondary btn-sm" onclick={retryRead}>Retry read</button>
							{/if}
						{/if}

						{#if inspector.stageError}<p class="text-sm text-error" role="alert">
								{inspector.stageError}
							</p>{/if}
						{#if inspector.result?.status === 'success' && inspector.result.value && resourceStageable}
							<button class="btn btn-secondary" onclick={useInChat}>
								<MessageSquarePlus class="size-4" aria-hidden="true" /> Use in Chat
							</button>
						{/if}
					</div>
				{:else}
					<div
						class="bg-base-200 dark:bg-base-300 rounded-lg p-6 text-center text-sm text-muted-content"
					>
						Select a resource to read and preview it.
					</div>
				{/if}
			</section>
		</div>
	{/if}
</div>

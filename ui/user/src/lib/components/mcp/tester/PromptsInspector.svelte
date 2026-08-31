<script lang="ts">
	import type { DirectOperationResult, MCPTesterSession } from '$lib/services/mcp/tester.svelte';
	import CapabilityList, { type CapabilityListItem } from './CapabilityList.svelte';
	import McpContent from './McpContent.svelte';
	import McpResult from './McpResult.svelte';
	import { Ban, MessageSquarePlus, Play } from '@lucide/svelte';

	interface Props {
		session: MCPTesterSession;
		onstaged: () => void;
	}

	let { session, onstaged }: Props = $props();
	let inspector = $derived(session.inspectors.prompts);
	let cache = $derived(session.cache.prompts);
	let selected = $derived(cache.items.find((prompt) => prompt.name === inspector.selectedName));
	let listItems = $derived<CapabilityListItem[]>(
		cache.items.map((prompt) => ({
			id: prompt.name,
			title: prompt.title || prompt.name,
			description: prompt.description,
			metadata: prompt.name
		}))
	);
	let requiredMissing = $derived(
		selected?.arguments?.some(
			(argument) => argument.required && !inspector.argumentsValue[argument.name]?.trim()
		) ?? false
	);
	let promptSupported = $derived(
		inspector.result?.value?.messages.every((message) => message.content.type === 'text') ?? false
	);
	let getActive = $derived(session.activeWorkflow?.label === 'prompt get');

	function selectPrompt(name: string) {
		inspector.selectedName = name;
		inspector.argumentsValue = {};
		inspector.result = undefined;
		inspector.stageError = undefined;
	}

	async function getPrompt() {
		if (!selected || requiredMissing) return;
		inspector.result = undefined;
		inspector.stageError = undefined;
		const args = Object.fromEntries(
			Object.entries(inspector.argumentsValue).filter(([, value]) => value.trim() !== '')
		);
		inspector.result = await session.getPrompt(selected.name, args);
	}

	function useInChat() {
		if (!selected || !inspector.result?.value) return;
		const staged = session.stagePrompt(selected.name, inspector.result.value);
		if (!staged.ok) {
			inspector.stageError = staged.message;
			return;
		}
		onstaged();
	}

	$effect(() => {
		if (!cache.loaded && !cache.loading && !cache.error && !session.activeWorkflow) {
			void session.loadPrompts();
		}
	});
</script>

<div class="flex h-full min-h-0 flex-col">
	<h2 class="sr-only">Prompts</h2>

	{#if cache.unsupported}
		<div class="bg-base-200 dark:bg-base-300 shrink-0 rounded-lg p-5" role="status">
			<h3 class="font-medium">Not supported</h3>
			<p class="mt-1 text-sm text-muted-content">This server does not advertise prompt support.</p>
		</div>
	{:else if cache.error && !cache.loading}
		<div class="notification-error mb-4 shrink-0 p-4" role="alert">
			<strong
				>{cache.errorStatus === 'cancelled'
					? 'Loading cancelled'
					: 'Prompts could not be loaded'}</strong
			>
			<p class="mt-1 text-sm">{cache.error}</p>
			<button class="btn btn-secondary btn-sm mt-3" onclick={() => session.loadPrompts(true)}
				>Retry</button
			>
		</div>
	{/if}

	{#if !cache.unsupported}
		<div
			class="default-scrollbar-thin grid min-h-0 flex-1 gap-6 overflow-y-auto md:grid-cols-[minmax(15rem,0.8fr)_minmax(0,1.7fr)] md:overflow-hidden"
		>
			<CapabilityList
				label="Prompts"
				items={listItems}
				selectedId={inspector.selectedName}
				loading={cache.loading}
				busy={Boolean(session.activeWorkflow)}
				onselect={selectPrompt}
				onrefresh={() => session.loadPrompts(true)}
				oncancel={() => session.cancelActiveWorkflow()}
			/>

			<section
				class="default-scrollbar-thin min-w-0 md:min-h-0 md:overflow-y-auto md:pr-1"
				aria-label="Prompt details"
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

						{#if selected.arguments?.length}
							<div class="space-y-3" aria-label="Prompt arguments">
								{#each selected.arguments as argument (argument.name)}
									<div class="space-y-1">
										<label
											for={`prompt-argument-${argument.name}`}
											class="block text-sm font-medium"
										>
											{argument.name}{argument.required ? ' *' : ''}
										</label>
										{#if argument.description}<p class="text-xs text-muted-content">
												{argument.description}
											</p>{/if}
										<input
											id={`prompt-argument-${argument.name}`}
											class="text-input-filled w-full"
											value={inspector.argumentsValue[argument.name] ?? ''}
											required={argument.required}
											oninput={(event) =>
												(inspector.argumentsValue = {
													...inspector.argumentsValue,
													[argument.name]: event.currentTarget.value
												})}
										/>
									</div>
								{/each}
							</div>
						{/if}

						<div class="flex flex-wrap gap-2">
							<button
								class="btn btn-primary"
								disabled={requiredMissing || Boolean(session.activeWorkflow)}
								onclick={getPrompt}
							>
								<Play class="size-4" aria-hidden="true" /> Get prompt
							</button>
							{#if getActive}
								<button class="btn btn-secondary" onclick={() => session.cancelActiveWorkflow()}>
									<Ban class="size-4" aria-hidden="true" /> Cancel
								</button>
							{/if}
						</div>

						{#if inspector.result?.value}
							<section class="space-y-3" aria-label="Resolved prompt preview">
								<h4 class="font-medium">Resolved messages</h4>
								{#each inspector.result.value.messages as message, index (index)}
									<div class="border-base-300 dark:border-base-400 rounded-lg border p-3">
										<p class="mb-2 text-xs font-semibold uppercase text-muted-content">
											{message.role}
										</p>
										<McpContent content={message.content} />
									</div>
								{/each}
								{#if !promptSupported}
									<p class="text-sm text-error" role="alert">
										This resolved prompt contains unsupported non-text content and cannot be staged.
									</p>
								{/if}
							</section>
						{/if}

						{#if inspector.result}
							<McpResult result={inspector.result as DirectOperationResult<unknown>} />
						{/if}

						{#if inspector.stageError}<p class="text-sm text-error" role="alert">
								{inspector.stageError}
							</p>{/if}
						{#if inspector.result?.status === 'success' && inspector.result.value && promptSupported}
							<button class="btn btn-secondary" onclick={useInChat}>
								<MessageSquarePlus class="size-4" aria-hidden="true" /> Use in Chat
							</button>
						{/if}
					</div>
				{:else}
					<div
						class="bg-base-200 dark:bg-base-300 rounded-lg p-6 text-center text-sm text-muted-content"
					>
						Select a prompt to provide arguments and preview it.
					</div>
				{/if}
			</section>
		</div>
	{/if}
</div>

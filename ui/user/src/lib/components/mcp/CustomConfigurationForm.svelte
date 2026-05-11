<script lang="ts">
	import type { LaunchServerType, MCPCatalogEntryFieldManifest } from '$lib/services';
	import { hasSecretBinding } from '$lib/services/chat/mcp';
	import Select from '../Select.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		readonly?: boolean;
		config?: MCPCatalogEntryFieldManifest[];
		secretBoundHeaders?: MCPCatalogEntryFieldManifest[];
		type?: LaunchServerType;
		isPrebuiltEntry?: boolean;
		overrideEnvField?: string[];
		overrideEnvTemplate?: Snippet<[{ config: MCPCatalogEntryFieldManifest; index: number }]>;
	}

	let {
		readonly,
		config = $bindable(),
		secretBoundHeaders,
		type,
		isPrebuiltEntry,
		overrideEnvField,
		overrideEnvTemplate
	}: Props = $props();

	// Separate secret-bound fields from user-configurable fields, preserving
	// original indices so bind:value still points at the right config slot.
	const indexedConfig = $derived((config ?? []).map((item, i) => ({ item, index: i })));
	const userConfig = $derived(indexedConfig.filter(({ item }) => !hasSecretBinding(item)));
	const secretBoundEnvs = $derived(indexedConfig.filter(({ item }) => hasSecretBinding(item)));
	const allSecretBound = $derived([
		...secretBoundEnvs.map(({ item }) => ({ item, source: 'env' as const })),
		...(secretBoundHeaders ?? []).map((item) => ({ item, source: 'header' as const }))
	]);
</script>

<!-- Environment Variables / Files Section -->
{#if !readonly || (readonly && userConfig.length > 0)}
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<h4 class="text-sm font-semibold">
			{type === 'single' ? 'User Supplied Configuration' : 'Configuration'}
		</h4>

		{#each userConfig as { item, index: i } (i)}
			{#if overrideEnvField?.includes(item.key) && overrideEnvTemplate}
				{@render overrideEnvTemplate({ config: config![i], index: i })}
			{:else}
				<div
					class="dark:border-surface3 bg-surface2 flex w-full items-center gap-4 rounded-lg border border-transparent p-4"
				>
					<div class="flex w-full flex-col gap-4">
						<div class="flex w-full flex-col gap-1">
							<label for={`env-type-${i}`} class="text-sm font-light">Type</label>
							<Select
								class="dark:border-surface3 bg-background border border-transparent"
								classes={{
									root: 'flex grow'
								}}
								options={[
									{ label: 'Environment Variable', id: 'environment_variable_type' },
									{ label: 'File', id: 'file_type' }
								]}
								disabled={readonly || isPrebuiltEntry}
								selected={config![i].file ? 'file_type' : 'environment_variable_type'}
								onSelect={(option) => {
									if (option.id === 'file_type') {
										config![i].file = true;
									} else {
										config![i].file = false;
									}
								}}
								id={`env-type-${i}`}
							/>
						</div>

						<p class="text-on-surface1 text-xs font-light">
							{#if config![i].file}
								The value {type === 'single' ? 'the user supplies' : 'you provide'} will be written to
								a file. An environment variable will be created using the name you specify in the Key
								field and its value will be the path to that file. This environment variable will be set
								inside your MCP server and you can reference it in the arguments section above using the
								syntax ${'{KEY_NAME}'}.
							{:else}
								{type === 'single' ? 'The value the user supplies' : 'The value you provide'} will be
								set as an environment variable using the name you specify in the Key field. This environment
								variable will be set inside your MCP server and you can reference it in the arguments
								section above using the syntax ${'{KEY_NAME}'}.
							{/if}
						</p>

						{#if type === 'single'}
							<p class="text-on-surface1 text-xs font-light">
								The Name and Description fields will be displayed to the user when configuring this
								server. The Key field will not.
							</p>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-name-${i}`} class="text-sm font-light">Name</label>
								<input
									id={`env-name-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config![i].name}
									disabled={readonly || isPrebuiltEntry}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-description-${i}`} class="text-sm font-light">Description</label>
								<input
									id={`env-description-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config![i].description}
									disabled={readonly || isPrebuiltEntry}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-key-${i}`} class="text-sm font-light">Key</label>
								<input
									id={`env-key-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config![i].key}
									placeholder="e.g. CUSTOM_API_KEY"
									disabled={readonly || isPrebuiltEntry}
								/>
							</div>
							<div class="flex gap-8">
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										bind:checked={config![i].sensitive}
										disabled={readonly || isPrebuiltEntry}
									/>
									<span class="text-sm">Sensitive</span>
								</label>
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										bind:checked={config![i].required}
										disabled={readonly || isPrebuiltEntry}
									/>
									<span class="text-sm">Required</span>
								</label>
							</div>
						{:else}
							<div class="flex w-full flex-col gap-1">
								<label for={`env-key-${i}`} class="text-sm font-light">Key</label>
								<input
									id={`env-key-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config![i].key}
									placeholder="e.g. CUSTOM_API_KEY"
									disabled={readonly || isPrebuiltEntry}
								/>
								{#if isPrebuiltEntry && config![i].description}
									<p class="text-on-surface1 text-xs font-light break-all">
										{config![i].description}
									</p>
								{/if}
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-value-${i}`} class="text-sm font-light">Value</label>
								{#if config![i].file}
									<textarea
										id={`env-value-${i}`}
										class="text-input-filled bg-background min-h-24 w-full resize-y shadow-none"
										bind:value={config![i].value}
										disabled={readonly}
										rows={(config![i].value ?? '').split('\n').length + 1}
									></textarea>
								{:else}
									<input
										id={`env-value-${i}`}
										class="text-input-filled bg-background w-full shadow-none"
										bind:value={config![i].value}
										placeholder="e.g. 123abcdef456"
										disabled={readonly}
										type={config![i].sensitive ? 'password' : 'text'}
									/>
								{/if}
							</div>
							<div class="flex w-full gap-4">
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										bind:checked={config![i].sensitive}
										disabled={readonly || isPrebuiltEntry}
									/>
									<span class="text-sm">Sensitive</span>
								</label>
							</div>
						{/if}
					</div>
					{#if !readonly && !isPrebuiltEntry}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								config!.splice(i, 1);
							}}
							disabled={isPrebuiltEntry}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				</div>
			{/if}
		{/each}

		{#if !readonly && !isPrebuiltEntry}
			<div class="flex justify-end">
				<button
					class="button flex items-center gap-1 text-xs"
					onclick={() => {
						if (config) {
							config.push({
								key: '',
								description: '',
								name: '',
								value: '',
								required: false,
								sensitive: false,
								file: false
							});
						}
					}}
				>
					<Plus class="size-4" />
					{type === 'single' ? 'User Configuration' : 'Configuration'}
				</button>
			</div>
		{/if}
	</div>
{/if}

<!-- Secret-bound Configuration Section -->
{#if allSecretBound.length > 0}
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<h4 class="text-sm font-semibold">Secret-bound Configuration</h4>

		{#each allSecretBound as { item, source } (`${source}:${item.key}`)}
			<div
				class="dark:border-surface3 bg-surface2 flex w-full items-center gap-4 rounded-lg border border-transparent p-4"
			>
				<div class="flex w-full flex-col gap-4">
					<div class="flex w-full flex-col gap-1">
						<label class="text-sm font-light">Type</label>
						<span class="text-sm"
							>{source === 'header' ? 'Header' : item.file ? 'File' : 'Environment Variable'}</span
						>
					</div>

					<div class="flex w-full flex-col gap-1">
						<label class="text-sm font-light">Name</label>
						<span class="text-sm">{item.name || item.key}</span>
					</div>

					{#if item.description}
						<div class="flex w-full flex-col gap-1">
							<label class="text-sm font-light">Description</label>
							<span class="text-sm">{item.description}</span>
						</div>
					{/if}

					<div class="flex w-full flex-col gap-1">
						<label class="text-sm font-light">Key</label>
						<span class="text-sm font-mono">{item.key}</span>
					</div>

					<div class="flex w-full flex-col gap-1">
						<label class="text-sm font-light">Secret</label>
						<span class="text-sm">
							<code class="font-mono">{item.secretBinding?.name}</code> /
							<code class="font-mono">{item.secretBinding?.key}</code>
						</span>
					</div>

					<div class="flex flex-wrap gap-2">
						{#if item.sensitive}
							<span
								class="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-700 dark:bg-gray-700 dark:text-gray-300"
							>
								sensitive
							</span>
						{/if}
						{#if item.required}
							<span
								class="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-700 dark:bg-gray-700 dark:text-gray-300"
							>
								required
							</span>
						{/if}
						{#if source === 'env' && item.file}
							<span
								class="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-700 dark:bg-gray-700 dark:text-gray-300"
							>
								file
							</span>
						{/if}
						{#if source === 'env' && item.dynamicFile}
							<span
								class="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-700 dark:bg-gray-700 dark:text-gray-300"
							>
								dynamic
							</span>
						{/if}
					</div>
				</div>
			</div>
		{/each}
	</div>
{/if}

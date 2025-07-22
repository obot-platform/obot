<script lang="ts">
	import type {
		RemoteCatalogConfigAdmin,
		RemoteRuntimeConfigAdmin
	} from '$lib/services/admin/types';
	import { Plus, Trash2 } from 'lucide-svelte';
	import Select from '../Select.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { fade, slide } from 'svelte/transition';
	import Toggle from '../Toggle.svelte';

	interface Props {
		config: RemoteCatalogConfigAdmin | RemoteRuntimeConfigAdmin;
		readonly?: boolean;
	}
	let { config = $bindable(), readonly }: Props = $props();

	// For catalog entries, we show advanced config if hostname or headers exist
	// For servers, we always show the URL field (no advanced toggle needed)
	let showAdvanced = $state(
		Boolean(
			(config as RemoteCatalogConfigAdmin).hostname || (config.headers && config.headers.length > 0)
		)
	);

	let selectedType = $state<'fixedURL' | 'hostname'>(
		(config as RemoteCatalogConfigAdmin).hostname &&
			(config as RemoteCatalogConfigAdmin).hostname!.length > 0
			? 'hostname'
			: 'fixedURL'
	);
</script>

{#if !showAdvanced}
	{@const remoteConfig = config as RemoteCatalogConfigAdmin}
	<!-- For catalog entries, show simple fixed URL when not in advanced mode -->
	<div
		class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
		in:fade={{ duration: 200 }}
	>
		<h4 class="w-24 text-sm font-light">URL</h4>
		<input
			class="text-input-filled flex grow dark:bg-black"
			bind:value={remoteConfig.fixedURL}
			disabled={readonly || showAdvanced}
		/>
	</div>
{/if}

{#if showAdvanced}
	<div class="flex w-full flex-col gap-8" in:slide>
		<div
			class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
		>
			<div class="flex items-center gap-4 {readonly ? 'hidden' : ''}">
				<label for="remote-type" class="flex-shrink-0 text-sm font-light"
					>Restrict connections to:</label
				>
				<Select
					class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner dark:bg-black"
					classes={{
						root: 'flex grow'
					}}
					options={[
						{ label: 'Exact URL', id: 'fixedURL' },
						{ label: 'Hostname', id: 'hostname' }
					]}
					selected={selectedType}
					onSelect={(option) => {
						const catalogConfig = config as RemoteCatalogConfigAdmin;
						if (option.id === 'fixedURL') {
							catalogConfig.hostname = undefined;
							selectedType = 'fixedURL';
							catalogConfig.fixedURL = '';
						} else {
							catalogConfig.fixedURL = undefined;
							catalogConfig.hostname = '';
							selectedType = 'hostname';
						}
					}}
				/>
			</div>
			{#if selectedType === 'fixedURL' && typeof (config as RemoteCatalogConfigAdmin).fixedURL !== 'undefined'}
				{@const remoteConfig = config as RemoteCatalogConfigAdmin}
				<div class="flex items-center gap-2">
					<label for="remote-url" class="min-w-18 text-sm font-light">Exact URL</label>
					<input
						class="text-input-filled flex grow dark:bg-black"
						bind:value={remoteConfig.fixedURL}
						disabled={readonly}
						placeholder="e.g. https://custom.mcpserver.example.com/go/to"
					/>
				</div>
			{:else if selectedType === 'hostname' && typeof (config as RemoteCatalogConfigAdmin).hostname !== 'undefined'}
				{@const remoteConfig = config as RemoteCatalogConfigAdmin}
				<div class="flex items-center gap-2">
					<label for="remote-url" class="min-w-18 text-sm font-light">Hostname</label>
					<input
						class="text-input-filled flex grow dark:bg-black"
						bind:value={remoteConfig.hostname}
						disabled={readonly}
						placeholder="e.g. mycustomdomain"
					/>
				</div>
			{/if}
		</div>
	</div>
	<div class="flex w-full flex-col gap-8" in:slide>
		<div
			class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
		>
			<h4 class="text-sm font-semibold">Headers</h4>
			<p class="text-xs font-light text-gray-400 dark:text-gray-600">
				Header values will be supplied with the URL to configure the MCP server. Their values will
				be supplied by the user during initial setup.
			</p>
			{#if config.headers}
				{#each config.headers as header, i (i)}
					<div
						class="dark:border-surface3 flex w-full items-center gap-4 rounded-lg border border-transparent bg-gray-50 p-4 dark:bg-gray-900"
					>
						<div class="flex w-full flex-col gap-4">
							<div class="flex w-full flex-col gap-1">
								<label for={`header-name-${i}`} class="text-sm font-light">Name</label>
								<input
									id={`header-name-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].name}
									disabled={readonly}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`header-description-${i}`} class="text-sm font-light">Description</label
								>
								<input
									id={`header-description-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].description}
									disabled={readonly}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`header-key-${i}`} class="text-sm font-light">Key</label>
								<input
									id={`header-key-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].key}
									placeholder="e.g. CUSTOM_HEADER_KEY"
									disabled={readonly}
								/>
							</div>
							<div class="flex gap-8">
								<Toggle
									classes={{ label: 'text-sm text-inherit' }}
									disabled={readonly}
									label="Sensitive"
									labelInline
									checked={!!header.sensitive}
									onChange={(checked) => {
										if (config.headers?.[i]) {
											config.headers[i].sensitive = checked;
										}
									}}
								/>
								<Toggle
									classes={{ label: 'text-sm text-inherit' }}
									disabled={readonly}
									label="Required"
									labelInline
									checked={!!header.required}
									onChange={(checked) => {
										if (config.headers?.[i]) {
											config.headers[i].required = checked;
										}
									}}
								/>
							</div>
						</div>

						{#if !readonly}
							<button
								class="icon-button"
								onclick={() => {
									config.headers?.splice(i, 1);
								}}
								use:tooltip={'Delete Header'}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
				{/each}
			{/if}
			{#if !readonly}
				<div class="flex justify-end">
					<button
						class="button flex items-center gap-1 text-xs"
						onclick={() =>
							config.headers?.push({
								key: '',
								description: '',
								name: '',
								value: '',
								required: false,
								sensitive: false
							})}
					>
						<Plus class="size-4" />
						Header
					</button>
				</div>
			{/if}
		</div>
	</div>
{/if}

<button
	class="button-text pl-0"
	onclick={() => {
		showAdvanced = !showAdvanced;

		if (!showAdvanced) {
			const catalogConfig = config as RemoteCatalogConfigAdmin;
			catalogConfig.hostname = undefined;
			catalogConfig.fixedURL = catalogConfig.fixedURL ?? '';
		}
	}}
>
	{showAdvanced ? 'Reset Default Configuration' : 'Advanced Configuration'}
</button>

<script lang="ts">
	import type { LaunchServerType, MCPCatalogEntryFieldManifest } from '$lib/services';
	import Select from '../Select.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';

	interface Props {
		readonly?: boolean;
		config?: MCPCatalogEntryFieldManifest[];
		type?: LaunchServerType;
		isPrebuiltEntry?: boolean;
	}

	let { readonly, config = $bindable(), type, isPrebuiltEntry }: Props = $props();
</script>

<!-- Environment Variables / Files Section -->
{#if !readonly || (readonly && config && config.length > 0)}
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<h4 class="text-sm font-semibold">
			{type === 'single' ? 'User Supplied Configuration' : 'Configuration'}
		</h4>

		{#if config}
			{#each config as _, i (i)}
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
								selected={config[i].file ? 'file_type' : 'environment_variable_type'}
								onSelect={(option) => {
									if (option.id === 'file_type') {
										config[i].file = true;
									} else {
										config[i].file = false;
									}
								}}
								id={`env-type-${i}`}
							/>
						</div>

						<p class="text-on-surface1 text-xs font-light">
							{#if config[i].file}
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
									bind:value={config[i].name}
									disabled={readonly || isPrebuiltEntry}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-description-${i}`} class="text-sm font-light">Description</label>
								<input
									id={`env-description-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config[i].description}
									disabled={readonly || isPrebuiltEntry}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-key-${i}`} class="text-sm font-light">Key</label>
								<input
									id={`env-key-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config[i].key}
									placeholder="e.g. CUSTOM_API_KEY"
									disabled={readonly || isPrebuiltEntry}
								/>
							</div>
							<div class="flex gap-8">
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										bind:checked={config[i].sensitive}
										disabled={readonly || isPrebuiltEntry}
									/>
									<span class="text-sm">Sensitive</span>
								</label>
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										bind:checked={config[i].required}
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
									bind:value={config[i].key}
									placeholder="e.g. CUSTOM_API_KEY"
									disabled={readonly || isPrebuiltEntry}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-value-${i}`} class="text-sm font-light">Value</label>
								{#if config[i].file}
									<textarea
										id={`env-value-${i}`}
										class="text-input-filled bg-background min-h-24 w-full resize-y shadow-none"
										bind:value={config[i].value}
										disabled={readonly}
										rows={(config[i].value ?? '').split('\n').length + 1}
									></textarea>
								{:else}
									<input
										id={`env-value-${i}`}
										class="text-input-filled bg-background w-full shadow-none"
										bind:value={config[i].value}
										placeholder="e.g. 123abcdef456"
										disabled={readonly}
										type={config[i].sensitive ? 'password' : 'text'}
									/>
								{/if}
							</div>
							<div class="flex w-full gap-4">
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										bind:checked={config[i].sensitive}
										disabled={readonly || isPrebuiltEntry}
									/>
									<span class="text-sm">Sensitive</span>
								</label>
							</div>
						{/if}
					</div>

					{#if !readonly}
						<button
							class="icon-button"
							onclick={() => {
								config.splice(i, 1);
							}}
							disabled={isPrebuiltEntry}
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
					disabled={isPrebuiltEntry}
				>
					<Plus class="size-4" />
					{type === 'single' ? 'User Configuration' : 'Configuration'}
				</button>
			</div>
		{/if}
	</div>
{/if}

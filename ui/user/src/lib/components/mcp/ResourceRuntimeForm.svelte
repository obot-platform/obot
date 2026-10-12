<script lang="ts">
	import { m } from '$lib/i18n';
	import type { MCPResourceRequirements, ResourceRuntimeConfig } from '$lib/services';

	interface Props {
		config: ResourceRuntimeConfig;
		readonly?: boolean;
		defaultResources?: MCPResourceRequirements;
	}

	let { config = $bindable(), readonly, defaultResources }: Props = $props();

	if (!config.requests) {
		config.requests = {
			cpu: '',
			memory: ''
		};
	}
	if (!config.limits) {
		config.limits = {
			cpu: '',
			memory: ''
		};
	}

	function defaultPlaceholder(value: string | undefined, example: string) {
		if (defaultResources) {
			return value || m.mcps_catalog_resources_none();
		}
		return example;
	}
</script>

<div class="paper">
	<h4 class="text-sm font-semibold">{m.mcps_catalog_resources_title()}</h4>
	<p class="text-xs text-muted-content">
		{m.mcps_catalog_resources_description_prefix()}<a
			href="https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits"
			class="text-link"
			rel="external noopener noreferrer"
			target="_blank">{m.mcps_catalog_resources_description_link()}</a
		>{m.mcps_catalog_resources_description_suffix()}
	</p>

	<div class="flex flex-col gap-3">
		<div class="flex items-center gap-1">
			<h5 class="text-xs font-semibold uppercase tracking-wide">
				{m.mcps_catalog_resources_cpu_settings()}
			</h5>
		</div>
		<div class="flex items-center gap-4 w-full">
			<div class="flex flex-col gap-1 flex-1">
				<label for="resource-requests-cpu" class="w-20 text-sm font-light">
					{m.mcps_catalog_resources_request()}
				</label>
				<input
					id="resource-requests-cpu"
					class="text-input-filled dark:bg-base-100 w-full"
					bind:value={config.requests!.cpu}
					disabled={readonly}
					placeholder={defaultPlaceholder(
						defaultResources?.requests?.cpu,
						m.mcps_example({ example: '10m' })
					)}
					onblur={() => {
						if (config.requests?.cpu) {
							config.requests.cpu = config.requests.cpu.trim();
						}
					}}
				/>
			</div>
			<div class="flex flex-col gap-1 flex-1">
				<label for="resource-limits-cpu" class="w-20 text-sm font-light">
					{m.mcps_catalog_resources_limit()}
				</label>
				<input
					id="resource-limits-cpu"
					class="text-input-filled dark:bg-base-100 w-full"
					bind:value={config.limits!.cpu}
					disabled={readonly}
					placeholder={defaultPlaceholder(
						defaultResources?.limits?.cpu,
						m.mcps_example({ example: '10m' })
					)}
					onblur={() => {
						if (config.limits?.cpu) {
							config.limits.cpu = config.limits.cpu.trim();
						}
					}}
				/>
			</div>
		</div>

		<div class="divider"></div>

		<div class="flex flex-col gap-3">
			<h5 class="text-xs font-semibold uppercase tracking-wide">
				{m.mcps_catalog_resources_memory_settings()}
			</h5>
			<div class="flex items-center gap-4 w-full">
				<div class="flex flex-col gap-1 flex-1">
					<label for="resource-requests-memory" class="w-20 text-sm font-light">
						{m.mcps_catalog_resources_request()}
					</label>
					<input
						id="resource-requests-memory"
						class="text-input-filled dark:bg-base-100 w-full"
						bind:value={config.requests!.memory}
						disabled={readonly}
						placeholder={defaultPlaceholder(
							defaultResources?.requests?.memory,
							m.mcps_example({ example: '200Mi' })
						)}
						onblur={() => {
							if (config.requests?.memory) {
								config.requests.memory = config.requests.memory.trim();
							}
						}}
					/>
				</div>
				<div class="flex flex-col gap-1 flex-1">
					<label for="resource-limits-memory" class="w-20 text-sm font-light">
						{m.mcps_catalog_resources_limit()}
					</label>
					<input
						id="resource-limits-memory"
						class="text-input-filled dark:bg-base-100 w-full"
						bind:value={config.limits!.memory}
						disabled={readonly}
						placeholder={defaultPlaceholder(
							defaultResources?.limits?.memory,
							m.mcps_example({ example: '200Mi' })
						)}
						onblur={() => {
							if (config.limits?.memory) {
								config.limits.memory = config.limits.memory.trim();
							}
						}}
					/>
				</div>
			</div>
		</div>
	</div>
</div>

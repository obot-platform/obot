<script lang="ts">
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
			return value || 'none';
		}
		return example;
	}
</script>

<div class="paper">
	<h4 class="text-sm font-semibold">Resource Limits & Requests</h4>
	<p class="text-xs text-muted-content">
		Define the CPU and memory requests and limits for deployments created by this MCP catalog entry.
		Leave fields blank to use the platform defaults shown in each field. See the Kubernetes <a
			href="https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits"
			class="text-link"
			rel="external"
			target="_blank">resource management documentation</a
		> for more information.
	</p>

	<div class="flex flex-col gap-3">
		<div class="flex items-center gap-1">
			<h5 class="text-xs font-semibold uppercase tracking-wide">CPU Settings</h5>
		</div>
		<div class="flex items-center gap-4 w-full">
			<div class="flex flex-col gap-1 flex-1">
				<label for="resource-requests-cpu" class="w-20 text-sm font-light"> Request </label>
				<input
					id="resource-requests-cpu"
					class="text-input-filled dark:bg-base-100 w-full"
					bind:value={config.requests!.cpu}
					disabled={readonly}
					placeholder={defaultPlaceholder(defaultResources?.requests?.cpu, 'e.g. 10m')}
					onblur={() => {
						if (config.requests?.cpu) {
							config.requests.cpu = config.requests.cpu.trim();
						}
					}}
				/>
			</div>
			<div class="flex flex-col gap-1 flex-1">
				<label for="resource-limits-cpu" class="w-20 text-sm font-light"> Limit </label>
				<input
					id="resource-limits-cpu"
					class="text-input-filled dark:bg-base-100 w-full"
					bind:value={config.limits!.cpu}
					disabled={readonly}
					placeholder={defaultPlaceholder(defaultResources?.limits?.cpu, 'e.g. 10m')}
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
			<h5 class="text-xs font-semibold uppercase tracking-wide">Memory Settings</h5>
			<div class="flex items-center gap-4 w-full">
				<div class="flex flex-col gap-1 flex-1">
					<label for="resource-requests-memory" class="w-20 text-sm font-light"> Request </label>
					<input
						id="resource-requests-memory"
						class="text-input-filled dark:bg-base-100 w-full"
						bind:value={config.requests!.memory}
						disabled={readonly}
						placeholder={defaultPlaceholder(defaultResources?.requests?.memory, 'e.g. 200Mi')}
						onblur={() => {
							if (config.requests?.memory) {
								config.requests.memory = config.requests.memory.trim();
							}
						}}
					/>
				</div>
				<div class="flex flex-col gap-1 flex-1">
					<label for="resource-limits-memory" class="w-20 text-sm font-light"> Limit </label>
					<input
						id="resource-limits-memory"
						class="text-input-filled dark:bg-base-100 w-full"
						bind:value={config.limits!.memory}
						disabled={readonly}
						placeholder={defaultPlaceholder(defaultResources?.limits?.memory, 'e.g. 200Mi')}
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

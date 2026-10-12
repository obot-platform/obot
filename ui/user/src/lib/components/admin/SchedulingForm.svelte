<script lang="ts">
	import { m } from '$lib/i18n';
	import { parseSchedulingResources } from '$lib/utils.js';
	import YamlEditor from './YamlEditor.svelte';
	import { Info, Lock } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	type SchedulingResources = ReturnType<typeof parseSchedulingResources>;

	interface Props {
		readonly?: boolean;
		locked?: boolean;
		maximumsLocked?: boolean;
		affinity?: string;
		tolerations?: string;
		runtimeClassName?: string;
		resourceInfo: SchedulingResources;
		children?: Snippet;
		notes?: Snippet;
		maximumResources?: Snippet;
		type: 'app' | 'mcpserver';
	}

	let {
		readonly,
		locked,
		affinity = $bindable(),
		tolerations = $bindable(),
		resourceInfo = $bindable(),
		runtimeClassName = $bindable(),
		children,
		notes,
		type,
		maximumResources,
		maximumsLocked
	}: Props = $props();
</script>

<div class="flex flex-col gap-2">
	{#if locked || maximumsLocked}
		<div class="notification-info p-3 text-sm font-light">
			<div class="flex items-center gap-3">
				<Info class="size-6" />
				<div>
					{maximumsLocked && !locked
						? m.platform_mcp_config_scheduling_helm_managed_max_prefix()
						: m.platform_mcp_config_scheduling_helm_managed_settings_prefix()}
					<b class="font-semibold">{m.platform_mcp_config_scheduling_read_only()}</b>
					{m.platform_mcp_config_scheduling_helm_managed_suffix()}
				</div>
			</div>
		</div>
	{/if}

	{#if notes}
		{@render notes()}
	{/if}
</div>

<div class="paper mt-1">
	<div>
		{@render headerContent(m.platform_mcp_config_scheduling_affinity())}
		<p class="text-sm">
			{m.platform_mcp_config_scheduling_affinity_desc_1({
				target:
					type === 'app'
						? m.platform_mcp_config_scheduling_target_application_deployment()
						: m.platform_mcp_config_scheduling_target_pods_every_mcp_deployment()
			})}
			<code>spec.template.spec.affinity</code>
			{m.platform_mcp_config_scheduling_affinity_desc_2()}
			<a
				class="text-link"
				href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#affinity-v1-core"
				rel="external noopener noreferrer"
				target="_blank">{m.platform_mcp_config_scheduling_affinity_object()}</a
			>{m.platform_mcp_config_scheduling_affinity_desc_3()}
			<a
				href="https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity"
				target="_blank"
				rel="external noopener noreferrer"
				class="text-link">{m.platform_mcp_config_scheduling_affinity_documentation()}</a
			>
			{m.platform_mcp_config_scheduling_see_for_more_details()}
		</p>
	</div>
	<div class="flex flex-col gap-1">
		<div class="text-sm font-light">
			{m.platform_mcp_config_scheduling_affinity_configuration()}
		</div>
		<YamlEditor bind:value={affinity} disabled={readonly} placeholder="" rows={6} autoHeight />
	</div>
</div>
<div class="paper mt-1">
	<div>
		{@render headerContent(m.platform_mcp_config_scheduling_tolerations())}
		<p class="text-sm">
			{m.platform_mcp_config_scheduling_tolerations_desc_1({
				target:
					type === 'app'
						? m.platform_mcp_config_scheduling_target_application_deployment()
						: m.platform_mcp_config_scheduling_target_pods_every_mcp_deployment()
			})}
			<code>spec.template.spec.tolerations</code>
			{m.platform_mcp_config_scheduling_tolerations_desc_2()}
			<a
				href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core"
				class="text-link"
				rel="external noopener noreferrer"
				target="_blank">{m.platform_mcp_config_scheduling_toleration_objects()}</a
			>{m.platform_mcp_config_scheduling_affinity_desc_3()}
			<a
				href="https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/"
				target="_blank"
				rel="external noopener noreferrer"
				class="text-link">{m.platform_mcp_config_scheduling_taints_tolerations_documentation()}</a
			>
			{m.platform_mcp_config_scheduling_see_for_more_details()}
		</p>
	</div>
	<div class="flex flex-col gap-1">
		<div class="text-sm font-light">
			{m.platform_mcp_config_scheduling_tolerations_configuration()}
		</div>
		<YamlEditor bind:value={tolerations} disabled={readonly} placeholder="" rows={6} autoHeight />
	</div>
</div>
<div class="paper mt-1">
	<div>
		{@render headerContent(m.platform_mcp_config_scheduling_resource_limits_requests())}
		<p class="text-sm">
			{m.platform_mcp_config_scheduling_resources_desc_1({
				target:
					type === 'app'
						? m.platform_mcp_config_scheduling_target_the_application_deployment()
						: m.platform_mcp_config_scheduling_target_pods_every_hosted_deployment()
			})}
			<a
				href="https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits"
				class="text-link"
				rel="external noopener noreferrer"
				target="_blank">{m.platform_mcp_config_scheduling_resource_management_documentation()}</a
			>
			{m.platform_mcp_config_scheduling_see_for_more_information()}
		</p>
	</div>

	<h3 class="text-base font-semibold">{m.platform_mcp_config_cpu_settings()}</h3>
	<div class="flex gap-4">
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="cpu-request"
				>{m.platform_mcp_config_scheduling_request()}</label
			>
			<input
				type="text"
				id="cpu-request"
				bind:value={resourceInfo.requests.cpu}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder={m.platform_example_value({ value: '500m' })}
			/>
		</div>
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="cpu-limit">{m.platform_mcp_config_scheduling_limit()}</label>
			<input
				type="text"
				id="cpu-limit"
				bind:value={resourceInfo.limits.cpu}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder={m.platform_example_value({ value: '1' })}
			/>
		</div>
	</div>
	<h3 class="text-base font-semibold">{m.platform_mcp_config_memory_settings()}</h3>
	<div class="flex gap-4">
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="memory-request"
				>{m.platform_mcp_config_scheduling_request()}</label
			>
			<input
				type="text"
				id="memory-request"
				bind:value={resourceInfo.requests.memory}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder={m.platform_example_value({ value: '512Mi' })}
			/>
		</div>
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="memory-limit"
				>{m.platform_mcp_config_scheduling_limit()}</label
			>
			<input
				type="text"
				id="memory-limit"
				bind:value={resourceInfo.limits.memory}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder={m.platform_example_value({ value: '1Gi' })}
			/>
		</div>
	</div>

	{#if maximumResources}
		<div class="divider my-0"></div>
		{@render maximumResources()}
	{/if}
</div>
<div class="paper mt-1">
	<div>
		{@render headerContent(m.platform_mcp_config_scheduling_runtime_class())}
		<p class="text-sm">
			{m.platform_mcp_config_scheduling_runtime_desc_1()}
			<a
				href="https://kubernetes.io/docs/concepts/containers/runtime-class/"
				class="text-link"
				rel="external noopener noreferrer"
				target="_blank">RuntimeClass</a
			>
			{m.platform_mcp_config_scheduling_runtime_desc_2({
				target:
					type === 'app'
						? m.platform_mcp_config_scheduling_target_the_application_deployment()
						: m.platform_mcp_config_scheduling_target_mcp_server_pods()
			})}
			<a
				href="https://gvisor.dev/"
				class="text-link"
				rel="external noopener noreferrer"
				target="_blank">gVisor</a
			>
			{m.platform_mcp_config_scheduling_or()}
			<a
				href="https://katacontainers.io/"
				class="text-link"
				rel="external noopener noreferrer"
				target="_blank">Kata Containers</a
			>
			{m.platform_mcp_config_scheduling_runtime_desc_3()}
		</p>
	</div>
	<div class="flex flex-col gap-1">
		<label class="input-label" for="runtime-class-name"
			>{m.platform_mcp_config_scheduling_runtime_class_name()}</label
		>
		<input
			type="text"
			id="runtime-class-name"
			bind:value={runtimeClassName}
			class="text-input-filled dark:bg-base-100"
			disabled={readonly}
			placeholder={m.platform_example_value({ value: 'gvisor' })}
		/>
		<p class="text-xs font-light text-muted-content">
			{m.platform_mcp_config_scheduling_runtime_leave_empty()}
		</p>
	</div>
</div>

{#if children}
	{@render children()}
{/if}

{#snippet headerContent(title: string)}
	<h2 class="text-lg font-semibold">
		{title}
		{#if locked}
			<span class="pill-rounded nowrap font-light">
				<Lock class="size-3" />
				{m.platform_mcp_config_scheduling_helm_deployed()}
			</span>
		{/if}
	</h2>
{/snippet}

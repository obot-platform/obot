<script lang="ts">
	import type { formatResources } from '$lib/format';
	import type { SharedK8sSettings } from '$lib/services';
	import YamlEditor from '../YamlEditor.svelte';
	import HelmDeployedSectionHeader from './ServerScheduleSectionTitle.svelte';
	import { Info } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		readonly?: boolean;
		children?: Snippet;
		notes?: Snippet;
		affinity?: SharedK8sSettings['affinity'];
		tolerations?: SharedK8sSettings['tolerations'];
		resourceInfo: ReturnType<typeof formatResources>;
		runtimeClassName?: SharedK8sSettings['runtimeClassName'];
		configType?: 'mcp' | 'obot';
	}

	let {
		readonly,
		children,
		notes,
		affinity = $bindable(),
		tolerations = $bindable(),
		resourceInfo = $bindable(),
		runtimeClassName = $bindable(),
		configType = 'obot'
	}: Props = $props();
</script>

<div class="flex flex-col gap-2">
	{#if readonly}
		<div class="notification-info p-3 text-sm font-light">
			<div class="flex items-center gap-3">
				<Info class="size-6" />
				<div>
					These settings are currently managed by your Helm chart and are <b class="font-semibold"
						>read-only</b
					> in the UI. To edit them, update your Helm values and redeploy.
				</div>
			</div>
		</div>
	{/if}

	{#if notes}
		{@render notes()}
	{/if}
</div>

<div class="paper">
	<div>
		<HelmDeployedSectionHeader title="Affinity" />
		<p class="text-sm">
			Define the affinity field for the pods {configType === 'mcp'
				? 'in every MCP deployment'
				: 'in the Obot deployment'}. This value will be used to set the
			<code>spec.template.spec.affinity</code>
			field on Kubernetes deployments and must be a valid
			<a
				class="text-link"
				href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#affinity-v1-core"
				rel="external"
				target="_blank">Affinity object</a
			>. See the Kubernetes
			<a
				href="https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity"
				target="_blank"
				rel="external"
				class="text-link">affinity documentation</a
			> for more details.
		</p>
	</div>
	<div class="flex flex-col gap-1">
		<div class="text-sm font-light">Affinity Configuration</div>
		<YamlEditor bind:value={affinity} disabled={readonly} placeholder="" rows={6} autoHeight />
	</div>
</div>
<div class="paper">
	<div>
		<HelmDeployedSectionHeader title="Tolerations" />
		<p class="text-sm">
			Define the tolerations field for the pods {configType === 'mcp'
				? 'in every MCP deployment'
				: 'in the Obot deployment'}. This value will be used to set the
			<code>spec.template.spec.tolerations</code>
			field on Kubernetes deployments and must be a valid list of
			<a
				href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core"
				class="text-link"
				rel="external"
				target="_blank">Toleration objects</a
			>. See the Kubernetes
			<a
				href="https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/"
				target="_blank"
				rel="external"
				class="text-link">taints and tolerations documentation</a
			> for more details.
		</p>
	</div>
	<div class="flex flex-col gap-1">
		<div class="text-sm font-light">Tolerations Configuration</div>
		<YamlEditor bind:value={tolerations} disabled={readonly} placeholder="" rows={6} autoHeight />
	</div>
</div>
<div class="paper">
	<div>
		<HelmDeployedSectionHeader title="Resource Limits & Requests" />
		<p class="text-sm">
			Define the CPU and memory requests and limits for pods in {configType === 'mcp'
				? 'every hosted single or multi-tenant deployment'
				: 'the Obot deployment'}. See the Kubernetes
			<a
				href="https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits"
				class="text-link"
				rel="external"
				target="_blank">resource management documentation</a
			> for more information.
		</p>
	</div>

	<h3 class="text-lg font-semibold">CPU Settings</h3>
	<div class="flex gap-4">
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="cpu-request">Request</label>
			<input
				type="text"
				id="cpu-request"
				bind:value={resourceInfo.requests.cpu}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder="example: 500m"
			/>
		</div>
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="cpu-limit">Limit</label>
			<input
				type="text"
				id="cpu-limit"
				bind:value={resourceInfo.limits.cpu}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder="example: 1"
			/>
		</div>
	</div>
	<h3 class="text-lg font-semibold">Memory Settings</h3>
	<div class="flex gap-4">
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="memory-request">Request</label>
			<input
				type="text"
				id="memory-request"
				bind:value={resourceInfo.requests.memory}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder="example: 512Mi"
			/>
		</div>
		<div class="flex flex-1 flex-col gap-1">
			<label class="input-label" for="memory-limit">Limit</label>
			<input
				type="text"
				id="memory-limit"
				bind:value={resourceInfo.limits.memory}
				class="text-input-filled dark:bg-base-100"
				disabled={readonly}
				placeholder="example: 1Gi"
			/>
		</div>
	</div>
</div>
<div class="paper">
	<div>
		<HelmDeployedSectionHeader title="Runtime Class" />
		<p class="text-sm">
			Specify a <a
				href="https://kubernetes.io/docs/concepts/containers/runtime-class/"
				class="text-link"
				rel="external"
				target="_blank">RuntimeClass</a
			>
			for {configType === 'mcp' ? 'MCP server pods' : 'the Obot deployment'}. RuntimeClass allows
			you to select a specific container runtime configuration for enhanced security isolation.
			Container runtimes like
			<a href="https://gvisor.dev/" class="text-link" rel="external" target="_blank">gVisor</a>
			or
			<a href="https://katacontainers.io/" class="text-link" rel="external" target="_blank"
				>Kata Containers</a
			> provide stronger isolation by adding an additional security boundary between the container and
			the host kernel.
		</p>
	</div>
	<div class="flex flex-col gap-1">
		<label class="input-label" for="runtime-class-name">RuntimeClass Name</label>
		<input
			type="text"
			id="runtime-class-name"
			bind:value={runtimeClassName}
			class="text-input-filled dark:bg-base-100"
			disabled={readonly}
			placeholder="example: gvisor"
		/>
		<p class="text-xs font-light text-muted-content">
			Leave empty to use the cluster's default container runtime.
		</p>
	</div>
</div>

{#if children}
	{@render children()}
{/if}

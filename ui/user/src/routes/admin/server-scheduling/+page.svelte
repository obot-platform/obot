<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import YamlEditor from '$lib/components/admin/YamlEditor.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { formatResources } from '$lib/format.js';
	import { Info } from '@lucide/svelte';
	import { Lock } from '@lucide/svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	let { k8sSettings } = $derived(data);
	let resourceInfo = $derived(formatResources(data.k8sSettings?.resources));

	$effect(() => {
		console.log(k8sSettings, resourceInfo);
	});

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout title="Server Scheduling">
	<div class="relative h-full w-full" transition:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<div>
						These settings are managed by your Helm chart and are <b class="font-semibold"
							>read-only</b
						> in the UI. To edit them, update your Helm values and redeploy.
					</div>
				</div>
			</div>

			<div class="paper">
				{@render headerContent('Configuration')}

				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Dev</div>
					<input class="text-input-filled" disabled value={k8sSettings?.dev || ''} />
				</div>
				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Image</div>
					<YamlEditor
						value={k8sSettings?.image || ''}
						disabled
						placeholder=""
						rows={3}
						autoHeight
					/>
				</div>
				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Ingress</div>
					<YamlEditor
						value={k8sSettings?.ingress || ''}
						disabled
						placeholder=""
						rows={6}
						autoHeight
					/>
				</div>

				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Service</div>
					<YamlEditor
						value={k8sSettings?.service || ''}
						disabled
						placeholder=""
						rows={3}
						autoHeight
					/>
				</div>

				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Service Account</div>
					<YamlEditor
						value={k8sSettings?.serviceAccount || ''}
						disabled
						placeholder=""
						rows={3}
						autoHeight
					/>
				</div>

				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Persistence</div>
					<YamlEditor
						value={k8sSettings?.persistence || ''}
						disabled
						placeholder=""
						rows={6}
						autoHeight
					/>
				</div>

				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Update Strategy</div>
					<input class="text-input-filled" disabled value={k8sSettings?.updateStrategy || ''} />
				</div>
			</div>

			<div class="paper">
				{@render headerContent('Configuration Variables')}
				<p class="text-sm">
					The configuration variables for the Obot Kubernetes deployment. See <a
						href="https://docs.obot.ai/configuration/server-configuration/"
						target="_blank"
						rel="external"
						class="text-link">Server Configuration</a
					> for more details.
				</p>
				<div class="flex flex-col gap-1">
					<div class="text-sm font-light">Config</div>
					<YamlEditor
						value={k8sSettings?.config || ''}
						disabled
						placeholder=""
						rows={6}
						autoHeight
					/>
				</div>
			</div>

			<div class="paper">
				<div>
					{@render headerContent('Affinity')}
					<p class="text-sm">
						Defines the affinity field for the Obot Kubernetes deployment.
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
					<YamlEditor
						value={k8sSettings?.affinity || ''}
						disabled
						placeholder=""
						rows={6}
						autoHeight
					/>
				</div>
			</div>
			<div class="paper">
				<div>
					{@render headerContent('Tolerations')}
					<p class="text-sm">
						Defines the tolerations field for the Obot Kubernetes deployment.
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
					<YamlEditor
						value={k8sSettings?.tolerations || ''}
						disabled
						placeholder=""
						rows={6}
						autoHeight
					/>
				</div>
			</div>

			<div class="paper">
				<div>
					{@render headerContent('Resource Limits & Requests')}
					<p class="text-sm">
						Defines the CPU and memory requests and limits for the Obot Kubernetes deployment. See
						the Kubernetes <a
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
							disabled
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
							disabled
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
							disabled
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
							disabled
							placeholder="example: 1Gi"
						/>
					</div>
				</div>
			</div>

			<div class="paper mt-1">
				<div>
					{@render headerContent('Runtime Class')}
					<p class="text-sm">
						Specifies <a
							href="https://kubernetes.io/docs/concepts/containers/runtime-class/"
							class="text-link"
							rel="external"
							target="_blank">RuntimeClass</a
						>
						for MCP server pods. RuntimeClass allows you to select a specific container runtime configuration
						for enhanced security isolation. Container runtimes like
						<a href="https://gvisor.dev/" class="text-link" rel="external" target="_blank">gVisor</a
						>
						or
						<a href="https://katacontainers.io/" class="text-link" rel="external" target="_blank"
							>Kata Containers</a
						> provide stronger isolation by adding an additional security boundary between the container
						and the host kernel.
					</p>
				</div>
				<div class="flex flex-col gap-1">
					<label class="input-label" for="runtime-class-name">RuntimeClass Name</label>
					<input
						type="text"
						id="runtime-class-name"
						value={k8sSettings?.runtimeClassName}
						class="text-input-filled dark:bg-base-100"
						disabled
						placeholder="example: gvisor"
					/>
				</div>
			</div>
		</div>
	</div>
</Layout>

{#snippet headerContent(title: string)}
	<h2 class="text-lg font-semibold">
		{title}
		<span class="pill-rounded nowrap font-light">
			<Lock class="size-3" /> Helm-Deployed
		</span>
	</h2>
{/snippet}

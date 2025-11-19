<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { Info, LoaderCircle, Lock } from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import { profile } from '$lib/stores/index.js';
	import { AdminService, type K8sSettings } from '$lib/services';
	import YamlEditor from '$lib/components/admin/YamlEditor.svelte';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let prevK8sSettings = $state(data.k8sSettings);
	let k8sSettings = $state<K8sSettings | undefined>({
		id: data.k8sSettings?.id ?? '',
		created: data.k8sSettings?.created ?? '',
		type: data.k8sSettings?.type ?? '',
		resources: data.k8sSettings?.resources ?? '',
		setViaHelm: data.k8sSettings?.setViaHelm ?? false,
		affinity: data.k8sSettings?.affinity ?? '',
		tolerations: data.k8sSettings?.tolerations ?? '',
		...data.k8sSettings
	});
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let resourceInfo = $state(convertResourcesForInput(data.k8sSettings?.resources));

	function stripQuotes(value: string): string {
		// Remove double quotes if the entire value is wrapped in them
		if (value.startsWith('"') && value.endsWith('"')) {
			return value.slice(1, -1);
		}
		return value;
	}

	function convertResourcesForInput(resources?: string) {
		if (!resources)
			return {
				requests: {
					cpu: '',
					memory: ''
				},
				limits: {
					cpu: '',
					memory: ''
				}
			};

		const result = {
			requests: {
				cpu: '',
				memory: ''
			},
			limits: {
				cpu: '',
				memory: ''
			}
		};

		const segments = resources.split('\n').map((segment) => segment.trim());
		const limitsIndex = segments.findIndex((segment) => segment.startsWith('limits:'));
		const requestsIndex = segments.findIndex((segment) => segment.startsWith('requests:'));

		if (requestsIndex !== -1) {
			const endIndex =
				limitsIndex !== -1 && limitsIndex > requestsIndex ? limitsIndex : segments.length;

			for (let i = requestsIndex + 1; i < endIndex; i++) {
				const line = segments[i];
				if (line.includes('cpu:')) {
					result.requests.cpu = stripQuotes(line.split('cpu:')[1]?.trim() ?? '');
				} else if (line.includes('memory:')) {
					result.requests.memory = stripQuotes(line.split('memory:')[1]?.trim() ?? '');
				}
			}
		}

		if (limitsIndex !== -1) {
			const endIndex =
				requestsIndex !== -1 && requestsIndex > limitsIndex ? requestsIndex : segments.length;

			for (let i = limitsIndex + 1; i < endIndex; i++) {
				const line = segments[i];
				if (line.includes('cpu:')) {
					result.limits.cpu = stripQuotes(line.split('cpu:')[1]?.trim() ?? '');
				} else if (line.includes('memory:')) {
					result.limits.memory = stripQuotes(line.split('memory:')[1]?.trim() ?? '');
				}
			}
		}

		return result;
	}

	function convertResourcesForOutput(output: ReturnType<typeof convertResourcesForInput>) {
		let outputString = '';
		if (output.requests.cpu || output.requests.memory) {
			outputString += `requests:\n  `;
			if (output.requests.cpu) {
				outputString += `cpu: ${output.requests.cpu.toString()}\n  `;
			}
			if (output.requests.memory) {
				outputString += `memory: ${output.requests.memory.toString()}\n`;
			}
		}

		if (output.limits.cpu || output.limits.memory) {
			outputString += `limits:\n  `;
			if (output.limits.cpu) {
				outputString += `cpu: ${output.limits.cpu.toString()}\n  `;
			}
			if (output.limits.memory) {
				outputString += `memory: ${output.limits.memory.toString()}\n`;
			}
		}

		return outputString;
	}

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	async function handleSave() {
		if (!k8sSettings) return;
		if (timeout) {
			clearTimeout(timeout);
		}
		saving = true;
		try {
			const response = await AdminService.updateK8sSettings({
				...k8sSettings,
				resources: convertResourcesForOutput(resourceInfo)
			});
			prevK8sSettings = k8sSettings;
			k8sSettings = response;
			resourceInfo = convertResourcesForInput(response.resources);
			showSaved = true;
			timeout = setTimeout(() => {
				showSaved = false;
			}, 3000);
		} catch (err) {
			console.error(err);
			// default behavior will show snackbar error
		} finally {
			saving = false;
		}
	}
</script>

<Layout classes={{ container: 'pb-0' }} title="Server Scheduling">
	<div class="relative mt-4 h-full w-full" transition:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			{#if k8sSettings}
				{@const readonly = k8sSettings?.setViaHelm || isAdminReadonly}
				<div class="flex flex-col gap-2">
					{#if k8sSettings?.setViaHelm}
						<div class="notification-info p-3 text-sm font-light">
							<div class="flex items-center gap-3">
								<Info class="size-6" />
								<div>
									These settings are currently managed by your Helm chart and are <b
										class="font-semibold">read-only</b
									> in the UI. To edit them, update your Helm values and redeploy.
								</div>
							</div>
						</div>
					{/if}

					<div class="notification-info p-3 text-sm font-light">
						<div class="flex items-center gap-2">
							<Info class="size-6" />
							<p class="text-md font-semibold">Configuration Notes</p>
						</div>
						<ul class="list-disc px-8 py-1 text-sm">
							<li>
								The below configuration maps directly to Kubernetes fields and functionality. <br />
								Links have been provided to the relevant Kubernetes documentation inline below.
							</li>
							<li>Resource configurations apply to all pods in the deployment.</li>
							<li>Changes will take effect on the next deployment or pod restart.</li>
							<li>Invalid YAML/JSON will be rejected during validation.</li>
						</ul>
					</div>
				</div>

				<div class="paper mt-1">
					<div>
						{@render headerContent('Affinity')}
						<p class="text-sm">
							Define the affinity field for the pods in every MCP deployment. This value will be
							used to set the <code>spec.template.spec.affinity</code> field on Kubernetes
							deployments and must be a valid
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
							bind:value={k8sSettings.affinity}
							disabled={readonly}
							placeholder=""
							rows={6}
							autoHeight
						/>
					</div>
				</div>
				<div class="paper mt-1">
					<div>
						{@render headerContent('Tolerations')}
						<p class="text-sm">
							Define the tolerations field for the pods in every MCP deployment. This value will be
							used to set the <code>spec.template.spec.tolerations</code> field on Kubernetes
							deployments and must be a valid list of
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
							bind:value={k8sSettings.tolerations}
							disabled={readonly}
							placeholder=""
							rows={6}
							autoHeight
						/>
					</div>
				</div>
				<div class="paper mt-1">
					<div>
						{@render headerContent('Resource Limits & Requests')}
						<p class="text-sm">
							Define the CPU and memory requests and limits for pods in every MCP deployment. See
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
								class="text-input-filled dark:bg-background"
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
								class="text-input-filled dark:bg-background"
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
								class="text-input-filled dark:bg-background"
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
								class="text-input-filled dark:bg-background"
								disabled={readonly}
								placeholder="example: 1Gi"
							/>
						</div>
					</div>
				</div>

				{#if !readonly}
					<div
						class="bg-surface1 dark:bg-background sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
					>
						{#if showSaved}
							<span
								in:fade={{ duration: 200 }}
								class="text-on-surface1 flex min-h-10 items-center px-4 text-sm font-extralight"
							>
								Your changes have been saved.
							</span>
						{/if}

						<button
							class="button hover:bg-surface3 flex items-center gap-1 bg-transparent"
							onclick={() => {
								k8sSettings = prevK8sSettings;
								resourceInfo = convertResourcesForInput(prevK8sSettings?.resources);
							}}
						>
							Reset
						</button>
						<button
							class="button-primary flex items-center gap-1"
							disabled={saving}
							onclick={handleSave}
						>
							{#if saving}
								<LoaderCircle class="size-4 animate-spin" />
							{:else}
								Save
							{/if}
						</button>
					</div>
				{:else}
					<div class="h-4"></div>
				{/if}
			{/if}
		</div>
	</div>
</Layout>

{#snippet headerContent(title: string)}
	<h2 class="text-lg font-semibold">
		{title}
		{#if k8sSettings?.setViaHelm}
			<span class="pill-rounded nowrap font-light">
				<Lock class="size-3" /> Helm-Deployed
			</span>
		{/if}
	</h2>
{/snippet}

<svelte:head>
	<title>Obot | Chat Configuration</title>
</svelte:head>

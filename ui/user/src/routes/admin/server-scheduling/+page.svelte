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

		const segments = resources.split('\n').map((segment) => segment.trim());
		const limitsIndex = segments.findIndex((segment) => segment.startsWith('limits:'));
		const requestsIndex = segments.findIndex((segment) => segment.startsWith('requests:'));
		return {
			requests: {
				cpu: stripQuotes(segments[requestsIndex + 1]?.split('cpu:')[1]?.trim() ?? ''),
				memory: stripQuotes(segments[requestsIndex + 2]?.split('memory:')[1]?.trim() ?? '')
			},
			limits: {
				cpu: stripQuotes(segments[limitsIndex + 1]?.split('cpu:')[1]?.trim() ?? ''),
				memory: stripQuotes(segments[limitsIndex + 2]?.split('memory:')[1]?.trim() ?? '')
			}
		};
	}

	function convertResourcesForOutput(output: ReturnType<typeof convertResourcesForInput>) {
		return `requests:\n  cpu: ${output.requests.cpu.toString()}\n  memory: ${output.requests.memory.toString()}\nlimits:\n  cpu: ${output.limits.cpu.toString()}\n  memory: ${output.limits.memory.toString()}`;
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

<Layout classes={{ container: 'pb-0' }}>
	<div class="relative mt-4 h-full w-full" transition:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="text-2xl font-semibold">Server Scheduling</h1>
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
							Define the affinity field for each pod in every MCP deployment. See <a
								href="https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity"
								target="_blank"
								rel="external"
								class="text-link">here</a
							> for more information.
						</p>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-sm font-light" for="affinity">Affinity Configuration</label>
						<YamlEditor
							bind:value={k8sSettings.affinity}
							disabled={readonly}
							placeholder=""
							rows={6}
						/>
					</div>
				</div>
				<div class="paper mt-1">
					<div>
						{@render headerContent('Tolerations')}
						<p class="text-sm">
							Define the tolerations field for each pod in every MCP server deployment. See <a
								href="https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/"
								target="_blank"
								rel="external"
								class="text-link">here</a
							> for more information.
						</p>
					</div>
					<div class="flex flex-col gap-1">
						<label class="text-sm font-light" for="tolerations">Tolerations Configuration</label>
						<YamlEditor
							bind:value={k8sSettings.tolerations}
							disabled={readonly}
							placeholder=""
							rows={6}
						/>
					</div>
				</div>
				<div class="paper mt-1">
					<div>
						{@render headerContent('Resource Limits & Requests')}
						<p class="text-sm">
							Set CPU memory requests and limits in the deployment. See <a
								href="https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits"
								class="text-link"
								rel="external"
								target="_blank">here</a
							> for more information.
						</p>
					</div>

					<h3 class="text-lg font-semibold">CPU Settings</h3>
					<div class="flex gap-4">
						<div class="flex flex-1 flex-col gap-1">
							<label class="input-label" for="description">Request</label>
							<input
								type="text"
								id="description"
								bind:value={resourceInfo.requests.cpu}
								class="text-input-filled dark:bg-black"
								disabled={readonly}
								placeholder="Default: Unset"
							/>
						</div>
						<div class="flex flex-1 flex-col gap-1">
							<label class="input-label" for="description">Limit</label>
							<input
								type="text"
								id="description"
								bind:value={resourceInfo.limits.cpu}
								class="text-input-filled dark:bg-black"
								disabled={readonly}
								placeholder="Default: Unset"
							/>
						</div>
					</div>
					<h3 class="text-lg font-semibold">Memory Settings</h3>
					<div class="flex gap-4">
						<div class="flex flex-1 flex-col gap-1">
							<label class="input-label" for="description">Request</label>
							<input
								type="text"
								id="description"
								bind:value={resourceInfo.requests.memory}
								class="text-input-filled dark:bg-black"
								disabled={readonly}
								placeholder="Default: 400Mi"
							/>
						</div>
						<div class="flex flex-1 flex-col gap-1">
							<label class="input-label" for="description">Limit</label>
							<input
								type="text"
								id="description"
								bind:value={resourceInfo.limits.memory}
								class="text-input-filled dark:bg-black"
								disabled={readonly}
								placeholder="Default: Unset"
							/>
						</div>
					</div>
				</div>

				{#if !readonly}
					<div
						class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
					>
						{#if showSaved}
							<span
								in:fade={{ duration: 200 }}
								class="flex min-h-10 items-center px-4 text-sm font-extralight text-gray-500"
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

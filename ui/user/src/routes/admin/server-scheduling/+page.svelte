<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea.js';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { Info, LoaderCircle } from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import { profile } from '$lib/stores/index.js';
	import { AdminService } from '$lib/services';
	import { limitShift } from '@floating-ui/dom';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let prevK8sSettings = $state(data.k8sSettings);
	let k8sSettings = $state(data.k8sSettings);
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let resourceInfo = $derived(convertResourcesForInput(data.k8sSettings?.resources));

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
				cpu: segments[requestsIndex + 1]?.split('cpu:')[1]?.trim() ?? '',
				memory: segments[requestsIndex + 2]?.split('memory:')[1]?.trim() ?? ''
			},
			limits: {
				cpu: segments[limitsIndex + 1]?.split('cpu:')[1]?.trim() ?? '',
				memory: segments[limitsIndex + 2]?.split('memory:')[1]?.trim() ?? ''
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
				<div class="flex flex-col gap-1">
					<div class="paper mt-1">
						<div>
							<h2 class="text-lg font-semibold">Pod Affinity</h2>
							<p>Define pod affinity and anti-affinity rules to control pod placement on nodes.</p>
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm" for="affinity">Affinity Configuration</label>
							<textarea
								id="affinity"
								rows={6}
								use:autoHeight
								bind:value={k8sSettings.affinity}
								class="text-input-filled dark:bg-black"
								disabled={readonly}
							>
							</textarea>
						</div>
					</div>
					<div class="paper mt-1">
						<div>
							<h2 class="text-xl font-semibold">Tolerations</h2>
							<p>Allow pods to schedule onto nodes with matching taints.</p>
						</div>
						<div class="flex flex-col gap-1">
							<label class="text-sm" for="tolerations">Tolerations Configuration</label>
							<textarea
								id="tolerations"
								rows={6}
								use:autoHeight
								bind:value={k8sSettings.tolerations}
								class="text-input-filled dark:bg-black"
								disabled={readonly}
							>
							</textarea>
						</div>
					</div>
				</div>
				<div class="flex flex-col gap-1">
					<h2 class="text-xl font-semibold">Resource Limits & Requests</h2>
					<p>Set CPU memory requests and limits in the deployment.</p>

					<div class="paper mt-1">
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
								/>
								<span class="input-description">Minimum CPU guaranteed (e.g. 500m, 1, 2)</span>
							</div>
							<div class="flex flex-1 flex-col gap-1">
								<label class="input-label" for="description">Limit</label>
								<input
									type="text"
									id="description"
									bind:value={resourceInfo.limits.cpu}
									class="text-input-filled dark:bg-black"
									disabled={readonly}
								/>
								<span class="input-description">Maximum CPU allowed (e.g. 1000m, 2, 4)</span>
							</div>
						</div>
					</div>
					<div class="paper mt-1">
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
								/>
								<span class="input-description">Minimum memory guaranteed (e.g. 256Mi, 1Gi)</span>
							</div>
							<div class="flex flex-1 flex-col gap-1">
								<label class="input-label" for="description">Limit</label>
								<input
									type="text"
									id="description"
									bind:value={resourceInfo.limits.memory}
									class="text-input-filled dark:bg-black"
									disabled={readonly}
								/>
								<span class="input-description">Maximum memory allowed (e.g. 1Gi, 4Gi)</span>
							</div>
						</div>
					</div>
				</div>
			{/if}

			{#if !isAdminReadonly}
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
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Chat Configuration</title>
</svelte:head>

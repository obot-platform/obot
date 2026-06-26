<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import ServerScheduleSectionTitle from '$lib/components/admin/server-scheduling/ServerScheduleSectionTitle.svelte';
	import ServerSchedulingForm from '$lib/components/admin/server-scheduling/ServerSchedulingForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { formatResources } from '$lib/format.js';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type K8sSettings } from '$lib/services';
	import { profile } from '$lib/stores/index.js';
	import { Info } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let prevK8sSettings = $state(untrack(() => data.k8sSettings));
	let k8sSettings = $state<K8sSettings | undefined>(
		untrack(() => ({
			id: data.k8sSettings?.id ?? '',
			created: data.k8sSettings?.created ?? '',
			type: data.k8sSettings?.type ?? '',
			resources: data.k8sSettings?.resources ?? '',
			setViaHelm: data.k8sSettings?.setViaHelm ?? false,
			affinity: data.k8sSettings?.affinity ?? '',
			tolerations: data.k8sSettings?.tolerations ?? '',
			runtimeClassName: data.k8sSettings?.runtimeClassName ?? '',
			storageClassName: data.k8sSettings?.storageClassName ?? '',
			nanobotWorkspaceSize: data.k8sSettings?.nanobotWorkspaceSize ?? '',
			...data.k8sSettings
		}))
	);
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let resourceInfo = $state(untrack(() => formatResources(data.k8sSettings?.resources)));

	function convertResourcesForOutput(output: ReturnType<typeof formatResources>) {
		let outputString = '';
		if (output.requests.cpu || output.requests.memory) {
			outputString += `requests:`;
			if (output.requests.cpu) {
				outputString += `\n  cpu: ${output.requests.cpu.toString()}`;
			}
			if (output.requests.memory) {
				outputString += `\n  memory: ${output.requests.memory.toString()}`;
			}
		}

		if (output.limits.cpu || output.limits.memory) {
			outputString += `\nlimits:`;
			if (output.limits.cpu) {
				outputString += `\n  cpu: ${output.limits.cpu.toString()}`;
			}
			if (output.limits.memory) {
				outputString += `\n  memory: ${output.limits.memory.toString()}`;
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
			const resources = convertResourcesForOutput(resourceInfo);
			const response = await AdminService.updateMcpServerK8sSettings({
				...k8sSettings,
				resources
			});
			prevK8sSettings = k8sSettings;
			k8sSettings = response;
			resourceInfo = formatResources(response.resources);
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

<Layout classes={{ container: 'pb-0' }} title="MCP Server Scheduling">
	<div class="relative h-full w-full" transition:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			{#if k8sSettings}
				{@const readonly = k8sSettings?.setViaHelm || isAdminReadonly}
				<ServerSchedulingForm
					{readonly}
					configType="mcp"
					bind:affinity={k8sSettings.affinity}
					bind:tolerations={k8sSettings.tolerations}
					bind:runtimeClassName={k8sSettings.runtimeClassName}
					bind:resourceInfo
				>
					{#snippet notes()}
						<div class="notification-info p-3 text-sm font-light">
							<div class="flex items-center gap-2">
								<Info class="size-6" />
								<p class="text-md font-semibold">Configuration Notes</p>
							</div>
							<ul class="list-disc px-8 py-1 text-sm">
								<li>
									The below configuration maps directly to Kubernetes fields and functionality. <br
									/>
									Links have been provided to the relevant Kubernetes documentation inline below.
								</li>
								<li>Resource configurations apply to all pods in the deployment.</li>
								<li>Changes will take effect on the next deployment or pod restart.</li>
								<li>Invalid YAML/JSON will be rejected during validation.</li>
							</ul>
						</div>
					{/snippet}

					<div class="paper">
						<div>
							<ServerScheduleSectionTitle title="Nanobot Workspace Storage" />
							<p class="text-sm">
								Configure the storage class and volume size used for nanobot workspace volumes.
								These values map to Kubernetes StorageClass configuration and persistent volume
								sizes. See the Kubernetes <a
									href="https://kubernetes.io/docs/concepts/storage/storage-classes/"
									class="text-link"
									rel="external"
									target="_blank">StorageClass documentation</a
								> for more details.
							</p>
						</div>
						<div class="flex flex-col gap-4">
							<div class="flex flex-col gap-1">
								<label class="input-label" for="storage-class-name">StorageClass Name</label>
								<input
									type="text"
									id="storage-class-name"
									bind:value={k8sSettings.storageClassName}
									class="text-input-filled dark:bg-base-100"
									disabled={readonly}
									placeholder="example: fast-ssd"
								/>
								<p class="text-xs font-light text-muted-content">
									Leave empty to use the cluster default StorageClass.
								</p>
							</div>
							<div class="flex flex-col gap-1">
								<label class="input-label" for="nanobot-workspace-size">Workspace Volume Size</label
								>
								<input
									type="text"
									id="nanobot-workspace-size"
									bind:value={k8sSettings.nanobotWorkspaceSize}
									class="text-input-filled dark:bg-base-100"
									disabled={readonly}
									placeholder="example: 10Gi"
								/>
								<p class="text-xs font-light text-muted-content">
									Use units like Gi or Mi (example: 10Gi, 512Mi).
								</p>
							</div>
						</div>
					</div>
				</ServerSchedulingForm>

				{#if !readonly}
					<div
						class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
					>
						{#if showSaved}
							<span
								in:fade={{ duration: 200 }}
								class="text-muted-content flex min-h-10 items-center px-4 text-sm font-extralight"
							>
								Your changes have been saved.
							</span>
						{/if}

						<button
							class="btn btn-secondary hover:bg-base-400 flex items-center gap-1 bg-transparent"
							onclick={() => {
								k8sSettings = prevK8sSettings;
								resourceInfo = formatResources(prevK8sSettings?.resources);
							}}
						>
							Reset
						</button>
						<button
							class="btn btn-primary flex items-center gap-1"
							disabled={saving}
							onclick={handleSave}
						>
							{#if saving}
								<Loading class="size-4" />
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

<svelte:head>
	<title>Obot | MCP Server Scheduling</title>
</svelte:head>

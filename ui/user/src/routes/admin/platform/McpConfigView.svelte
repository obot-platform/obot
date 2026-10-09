<script lang="ts">
	import SchedulingForm from '$lib/components/admin/SchedulingForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type K8sSettings } from '$lib/services';
	import { profile } from '$lib/stores/index.js';
	import { parseSchedulingResources } from '$lib/utils.js';
	import { Info, Lock } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;
	let { k8sSettings: initialK8sSettings }: { k8sSettings: K8sSettings | undefined } = $props();
	let prevK8sSettings = $state(untrack(() => initialK8sSettings));
	let k8sSettings = $state<K8sSettings | undefined>(
		untrack(() =>
			initialK8sSettings
				? {
						...initialK8sSettings,
						id: initialK8sSettings.id ?? '',
						created: initialK8sSettings.created ?? '',
						type: initialK8sSettings.type ?? '',
						resources: initialK8sSettings.resources ?? '',
						setViaHelm: initialK8sSettings.setViaHelm ?? false,
						maximumsSetViaHelm: initialK8sSettings.maximumsSetViaHelm ?? false,
						affinity: initialK8sSettings.affinity ?? '',
						tolerations: initialK8sSettings.tolerations ?? '',
						runtimeClassName: initialK8sSettings.runtimeClassName ?? '',
						maxCpuRequest: initialK8sSettings.maxCpuRequest ?? '',
						maxMemoryRequest: initialK8sSettings.maxMemoryRequest ?? '',
						maxCpuLimit: initialK8sSettings.maxCpuLimit ?? '',
						maxMemoryLimit: initialK8sSettings.maxMemoryLimit ?? ''
					}
				: undefined
		)
	);
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let resourceInfo = $state(untrack(() => parseSchedulingResources(initialK8sSettings?.resources)));

	function convertResourcesForOutput(output: ReturnType<typeof parseSchedulingResources>) {
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
	let schedulingReadonly = $derived(Boolean(k8sSettings?.setViaHelm || isAdminReadonly));
	let maximumsReadonly = $derived(Boolean(k8sSettings?.maximumsSetViaHelm || isAdminReadonly));
	let readonly = $derived(schedulingReadonly && maximumsReadonly);

	async function handleSave() {
		if (!k8sSettings) return;
		if (timeout) {
			clearTimeout(timeout);
		}
		saving = true;
		try {
			const resources = convertResourcesForOutput(resourceInfo);
			const response = await AdminService.updateK8sSettings({
				...k8sSettings,
				resources
			});
			prevK8sSettings = k8sSettings;
			k8sSettings = response;
			resourceInfo = parseSchedulingResources(response.resources);
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

<div class="relative h-full w-full" in:fade={{ duration }}>
	{#if k8sSettings}
		<div class="flex flex-col gap-8">
			<SchedulingForm
				readonly={schedulingReadonly}
				locked={k8sSettings.setViaHelm}
				maximumsLocked={k8sSettings.maximumsSetViaHelm}
				bind:resourceInfo
				bind:affinity={k8sSettings.affinity}
				bind:tolerations={k8sSettings.tolerations}
				bind:runtimeClassName={k8sSettings.runtimeClassName}
				type="mcpserver"
			>
				{#snippet notes()}
					<div class="notification-info p-3 text-sm font-light">
						<div class="flex items-center gap-2">
							<Info class="size-6" />
							<p class="text-md font-semibold">{m.platform_mcp_config_notes()}</p>
						</div>
						<ul class="list-disc px-8 py-1 text-sm">
							<li>
								{m.platform_mcp_config_note_maps()} <br />
								{m.platform_mcp_config_note_links()}
							</li>
							<li>{m.platform_mcp_config_note_pods()}</li>
							<li>{m.platform_mcp_config_note_effect()}</li>
							<li>{m.platform_mcp_config_note_invalid()}</li>
						</ul>
					</div>
				{/snippet}
				{#snippet maximumResources()}
					{#if k8sSettings}
						<div>
							{@render headerContent(m.platform_mcp_config_maximum_settings(), true)}
							<p class="text-sm">
								{m.platform_mcp_config_maximum_description()}
							</p>
						</div>
						<div class="flex flex-col gap-1">
							<h3 class="text-base font-semibold">{m.platform_mcp_config_cpu_settings()}</h3>
							<div class="grid grid-cols-2 gap-4">
								<div class="flex flex-1 flex-col gap-1 col-span-2 md:col-span-1">
									<label class="input-label" for="max-cpu-request"
										>{m.platform_mcp_config_max_request()}</label
									>
									<input
										type="text"
										id="max-cpu-request"
										bind:value={k8sSettings.maxCpuRequest}
										class="text-input-filled dark:bg-base-100"
										disabled={maximumsReadonly}
										placeholder={m.platform_example_value({ value: '500m' })}
									/>
								</div>
								<div class="flex flex-1 flex-col gap-1 col-span-2 md:col-span-1">
									<label class="input-label" for="max-cpu-limit"
										>{m.platform_mcp_config_max_limit()}</label
									>
									<input
										type="text"
										id="max-cpu-limit"
										bind:value={k8sSettings.maxCpuLimit}
										class="text-input-filled dark:bg-base-100"
										disabled={maximumsReadonly}
										placeholder={m.platform_example_value({ value: '1' })}
									/>
								</div>
							</div>
						</div>
						<div class="flex flex-col gap-1">
							<h3 class="text-base font-semibold">{m.platform_mcp_config_memory_settings()}</h3>
							<div class="grid grid-cols-2 gap-4">
								<div class="flex flex-1 flex-col gap-1 col-span-2 md:col-span-1">
									<label class="input-label" for="max-memory-request"
										>{m.platform_mcp_config_max_request()}</label
									>
									<input
										type="text"
										id="max-memory-request"
										bind:value={k8sSettings.maxMemoryRequest}
										class="text-input-filled dark:bg-base-100"
										disabled={maximumsReadonly}
										placeholder={m.platform_example_value({ value: '512Mi' })}
									/>
								</div>
								<div class="flex flex-1 flex-col gap-1 col-span-2 md:col-span-1">
									<label class="input-label" for="max-memory-limit"
										>{m.platform_mcp_config_max_limit()}</label
									>
									<input
										type="text"
										id="max-memory-limit"
										bind:value={k8sSettings.maxMemoryLimit}
										class="text-input-filled dark:bg-base-100"
										disabled={maximumsReadonly}
										placeholder={m.platform_example_value({ value: '1Gi' })}
									/>
								</div>
							</div>
						</div>
					{/if}
				{/snippet}
			</SchedulingForm>

			{#if !readonly}
				<div
					class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
				>
					{#if showSaved}
						<span
							in:fade={{ duration: 200 }}
							class="text-muted-content flex min-h-10 items-center px-4 text-sm font-extralight"
						>
							{m.core_changes_saved()}
						</span>
					{/if}

					<button
						class="btn btn-secondary hover:bg-base-400 flex items-center gap-1 bg-transparent"
						onclick={() => {
							k8sSettings = prevK8sSettings;
							resourceInfo = parseSchedulingResources(prevK8sSettings?.resources);
						}}
					>
						{m.core_reset_page()}
					</button>
					<button
						class="btn btn-primary flex items-center gap-1"
						disabled={saving}
						onclick={handleSave}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_save()}
						{/if}
					</button>
				</div>
			{:else}
				<div class="h-4"></div>
			{/if}
		</div>
	{:else}
		<p class="text-muted-content text-sm font-light">
			{m.platform_mcp_config_k8s_only()}
		</p>
	{/if}
</div>

{#snippet headerContent(title: string, isMaximumSetViaHelm?: boolean)}
	{@const isHelmDeployed = isMaximumSetViaHelm
		? k8sSettings?.maximumsSetViaHelm
		: k8sSettings?.setViaHelm}
	<h2 class="text-lg font-semibold">
		{title}
		{#if isHelmDeployed}
			<span class="pill-rounded nowrap font-light">
				<Lock class="size-3" />
				{m.platform_mcp_config_helm_deployed()}
			</span>
		{/if}
	</h2>
{/snippet}

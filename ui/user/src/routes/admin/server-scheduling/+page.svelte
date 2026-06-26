<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import YamlEditor from '$lib/components/admin/YamlEditor.svelte';
	import ServerScheduleSectionTitle from '$lib/components/admin/server-scheduling/ServerScheduleSectionTitle.svelte';
	import ServerSchedulingForm from '$lib/components/admin/server-scheduling/ServerSchedulingForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { formatResources } from '$lib/format.js';
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
			{#if k8sSettings}
				<ServerSchedulingForm
					readonly
					bind:affinity={k8sSettings.affinity}
					bind:tolerations={k8sSettings.tolerations}
					bind:resourceInfo
					runtimeClassName={k8sSettings.runtimeClassName}
				>
					<div class="paper">
						<ServerScheduleSectionTitle title="Configuration" />

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
						<ServerScheduleSectionTitle title="Configuration Variables" />
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
				</ServerSchedulingForm>
			{/if}
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Server Scheduling</title>
</svelte:head>

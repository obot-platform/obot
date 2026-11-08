<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import ContentTaggingSettings from '$lib/components/supervision/ContentTaggingSettings.svelte';
	import LabelsOverview from '$lib/components/supervision/LabelsOverview.svelte';
	import ToolLabelReview from '$lib/components/supervision/ToolLabelReview.svelte';
	import { m } from '$lib/i18n';
	import type { ToolLabelResourceType } from '$lib/services/provenance';
	import { goto } from '$lib/url';

	let resourceType = $derived(page.url.searchParams.get('resourceType'));
	let resourceID = $derived(page.url.searchParams.get('resourceID') ?? '');
	let valid = $derived(resourceType === 'catalog-entry' || resourceType === 'vmcp');
	let overview = $derived(!resourceType && !resourceID);
	let title = $derived(overview ? m.supervision_labels_title() : m.supervision_tool_labels_title());
</script>

<svelte:head><title>Obot | {title}</title></svelte:head>

<Layout
	{title}
	showBackButton={!overview}
	onBackButtonClick={() =>
		goto(
			valid && resourceID
				? resourceType === 'vmcp'
					? `/vmcps/${resourceID}`
					: `/mcp-servers/c/${resourceID}?view=tools`
				: '/supervision'
		)}
>
	{#if overview}
		<div class="flex flex-col gap-6">
			<LabelsOverview />
			<ContentTaggingSettings />
		</div>
	{:else if valid && resourceID}
		<ToolLabelReview resourceType={resourceType as ToolLabelResourceType} {resourceID} />
	{:else}
		<p class="notification-info rounded-md p-4 text-sm">
			{m.supervision_tool_labels_choose_resource()}
		</p>
	{/if}
</Layout>

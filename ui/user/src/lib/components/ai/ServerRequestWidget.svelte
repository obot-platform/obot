<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { listVMCPs, type VMCP } from '$lib/services';
	import { getMCPServerRequest, type MCPServerRequest } from '$lib/services/mcpserverrequests';
	import { profile } from '$lib/stores';
	import ServerRequestCard from './ServerRequestCard.svelte';
	import WidgetFrame from './WidgetFrame.svelte';
	import { onMount } from 'svelte';

	let { requestId }: { requestId: string } = $props();

	let request = $state<MCPServerRequest>();
	let vmcps = $state<VMCP[]>([]);
	let error = $state('');
	let loading = $state(true);

	onMount(async () => {
		try {
			request = await getMCPServerRequest(requestId, { dontLogErrors: true });
			if (profile.current.isAdmin?.()) vmcps = await sharedVMCPs();
		} catch (e) {
			const parsed = parseErrorContent(e);
			error = parsed.status === 404 ? m.ai_widget_not_found() : parsed.message;
		} finally {
			loading = false;
		}
	});

	async function sharedVMCPs() {
		try {
			const all = await listVMCPs({ dontLogErrors: true });
			return all.filter((v) => !v.userID && !v.sourceURL);
		} catch {
			return [];
		}
	}
</script>

<WidgetFrame {loading} {error}>
	{#if request}
		<ServerRequestCard {request} {vmcps} />
	{/if}
</WidgetFrame>

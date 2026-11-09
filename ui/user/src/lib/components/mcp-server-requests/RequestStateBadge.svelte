<script lang="ts">
	import { m } from '$lib/i18n';
	import type { MCPServerRequestState } from '$lib/services/mcpserverrequests';

	let { state }: { state: MCPServerRequestState } = $props();

	const labels: Record<MCPServerRequestState, () => string> = {
		pending: m.server_requests_state_pending,
		approved: m.server_requests_state_approved,
		denied: m.server_requests_state_denied
	};
	const styles: Record<MCPServerRequestState, string> = {
		pending: 'bg-yellow-500/20 text-gray-800 dark:text-gray-100',
		approved: 'bg-success/15 text-success',
		denied: 'bg-red-500/15 text-red-700 dark:text-red-300'
	};
</script>

<span class="rounded-full px-2.5 py-0.5 text-xs font-medium {styles[state]}">{labels[state]()}</span
>

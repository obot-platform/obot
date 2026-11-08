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
		pending: 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-300',
		approved: 'bg-green-500/15 text-green-700 dark:text-green-300',
		denied: 'bg-red-500/15 text-red-700 dark:text-red-300'
	};
</script>

<span class="rounded-full px-2.5 py-0.5 text-xs font-medium {styles[state]}">{labels[state]()}</span
>

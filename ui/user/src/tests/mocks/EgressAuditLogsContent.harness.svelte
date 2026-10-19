<script lang="ts">
	import EgressAuditLogsContent from '$lib/components/admin/audit-logs/EgressAuditLogsContent.svelte';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';
</script>

<VirtualPageRoot itemHeight={56} overscan={5}>
	<EgressAuditLogsContent />
</VirtualPageRoot>

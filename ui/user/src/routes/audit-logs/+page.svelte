<script lang="ts">
	import { fade } from 'svelte/transition';
	import Layout from '$lib/components/Layout.svelte';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
</script>

<Layout
	classes={{ container: '', childrenContainer: 'max-w-none' }}
	showUserLinks
	title="Audit Logs"
>
	<div class="flex flex-col gap-8 pb-8" in:fade>
		<VirtualPageRoot class="flex w-full flex-col gap-4" itemHeight={56} overscan={5}>
			<AuditLogsPageContent
				mcpId={null}
				mcpServerCatalogEntryName={null}
				mcpServerDisplayName={null}
			/>
		</VirtualPageRoot>
	</div>
</Layout>

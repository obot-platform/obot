<script lang="ts">
	import ExportsPage from '$lib/components/admin/audit-log-exports/ExportsPage.svelte';
	import { m } from '$lib/i18n';
</script>

<ExportsPage
	logType="admin"
	title={m.audit_usage_audit_logs_admin_exports_title()}
	pageTitle={m.audit_usage_audit_logs_admin_exports_page_title()}
/>

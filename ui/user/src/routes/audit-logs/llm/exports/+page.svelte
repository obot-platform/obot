<script lang="ts">
	import ExportsPage from '$lib/components/admin/audit-log-exports/ExportsPage.svelte';
	import { m } from '$lib/i18n';
</script>

<ExportsPage
	logType="llm"
	title={m.audit_usage_audit_logs_llm_exports_title()}
	pageTitle={m.audit_usage_audit_logs_llm_exports_page_title()}
/>

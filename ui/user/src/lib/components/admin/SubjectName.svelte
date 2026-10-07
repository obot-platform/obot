<script lang="ts">
	import { m } from '$lib/i18n';
	import type { SubjectTableRow } from '$lib/subjectResolver';

	interface Props {
		subject: SubjectTableRow;
	}

	let { subject }: Props = $props();
</script>

<!-- The name of a policy's subject, marked when it is a disabled user. -->
<span class="flex items-center gap-2">
	{subject.displayName}
	{#if subject.disabled}
		<span class="pill-warning">{m.core_status_disabled()}</span>
	{/if}
</span>

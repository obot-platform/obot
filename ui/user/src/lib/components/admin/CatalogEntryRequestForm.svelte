<script lang="ts">
	import McpServerEntryForm from './McpServerEntryForm.svelte';
	import type { ComponentProps } from 'svelte';

	let props: ComponentProps<typeof McpServerEntryForm> = $props();
</script>

<!-- A new request gets its own confirmation, dialogs, discovery result and delivery state. -->
{#key props.toolListRequestID}
	<McpServerEntryForm {...props} />
{/key}

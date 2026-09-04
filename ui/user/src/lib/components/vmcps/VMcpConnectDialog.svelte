<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { VMCP } from '$lib/services';
	import { vmcpConnectURL } from '$lib/services/vmcps/utils';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let vmcp = $state<VMCP>();

	export function open(value: VMCP) {
		vmcp = value;
		dialog?.open();
	}

	function close() {
		vmcp = undefined;
		dialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={`Connect to ${vmcp?.displayName ?? 'vMCP'}`}
	onClose={close}
>
	{#if vmcp}
		<p class="text-muted-content mb-4 text-sm font-light">
			Use this URL in an MCP client. Each user connects directly to the vMCP endpoint.
		</p>
		<div class="relative">
			<input
				class="text-input-filled w-full pr-12 font-mono text-xs"
				readonly
				value={vmcpConnectURL(vmcp)}
			/>
			<div class="absolute top-1/2 right-1 -translate-y-1/2">
				<CopyButton text={vmcpConnectURL(vmcp)} noButtonText tooltipText="Copy Connect URL" />
			</div>
		</div>
	{/if}
</ResponsiveDialog>

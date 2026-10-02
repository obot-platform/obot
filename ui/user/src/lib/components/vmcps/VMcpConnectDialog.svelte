<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
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
	title={m.vmcps_connect_to_named({ name: vmcp?.displayName ?? 'vMCP' })}
	onClose={close}
>
	{#if vmcp && vmcpConnectURL(vmcp)}
		<p class="text-muted-content mb-4 text-sm font-light">
			{m.vmcps_connect_dialog_description()}
		</p>
		<div class="relative">
			<input
				class="text-input-filled w-full pr-12 font-mono text-xs"
				readonly
				value={vmcpConnectURL(vmcp)}
			/>
			<div class="absolute top-1/2 right-1 -translate-y-1/2">
				<CopyButton
					text={vmcpConnectURL(vmcp)}
					noButtonText
					tooltipText={m.vmcps_copy_connect_url()}
				/>
			</div>
		</div>
	{:else}
		<p class="text-muted-content text-sm">{m.vmcps_not_ready_to_connect()}</p>
	{/if}
</ResponsiveDialog>

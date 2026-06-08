<script lang="ts">
	import { MCP_CONNECTION_INVALID_LICENSE_MESSAGE } from '$lib/services/user/constants';
	import { license, profile } from '$lib/stores';
	import LicenseDowngradeDialog from './LicenseDowngradeDialog.svelte';
	import { ShieldAlert } from 'lucide-svelte';

	let downgradeDialog = $state<ReturnType<typeof LicenseDowngradeDialog>>();
	let licenseKey = $derived(license.current.licenseKey);
</script>

<div class="bg-base-100">
	<div class="bg-warning/10 text-warning px-4 py-2 flex justify-between md:justify-center gap-2">
		<div class="flex items-center gap-4 md:gap-0.5 justify-center">
			<ShieldAlert class="text-warning size-4 shrink-0" />
			<p class="text-xs">
				{#if profile.current.hasAdminAccess?.()}
					Your license is <b class="font-semibold uppercase">{licenseKey ? 'invalid' : 'missing'}</b
					>. For full functionality, it is recommended to resolve the outstanding issues.
				{:else}
					{MCP_CONNECTION_INVALID_LICENSE_MESSAGE}
				{/if}
			</p>
		</div>
		{#if profile.current.hasAdminAccess?.()}
			<button class="btn btn-xs btn-warning" onclick={() => downgradeDialog?.open()}>
				Resolve
			</button>
		{/if}
	</div>
</div>

<LicenseDowngradeDialog bind:this={downgradeDialog} />

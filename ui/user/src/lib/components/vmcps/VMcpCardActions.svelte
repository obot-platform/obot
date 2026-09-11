<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { MCP_CONNECTION_INVALID_LICENSE_MESSAGE } from '$lib/services/user/constants';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { profile, version, vmcpInstances } from '$lib/stores';
	import { goto } from '$lib/url';
	import { MessageCircle } from '@lucide/svelte';

	interface Props {
		connectURL?: string;
		connectButtonId?: string;
		onConnect?: (options?: VMcpConnectOptions) => void;
		id: string;
	}

	let { connectURL, connectButtonId, onConnect, id }: Props = $props();

	let hasLicenseEntitlementViolations = $derived(
		(version.current.licenseEntitlementViolations || []).length > 0
	);
	let hasConfiguredInstance = $derived(
		vmcpInstances.current.items.some(
			(candidate) =>
				candidate.vmcpID === id &&
				candidate.userID === profile.current.id &&
				candidate.status?.configured === true
		)
	);

	function goToTester() {
		goto(`/vmcps/test/${id}`);
	}

	function handleTest() {
		if (hasConfiguredInstance) {
			goToTester();
			return;
		}
		onConnect?.({ onConnected: goToTester });
	}
</script>

<div class="flex items-center gap-2">
	<div
		use:tooltip={{
			text: hasLicenseEntitlementViolations ? MCP_CONNECTION_INVALID_LICENSE_MESSAGE : undefined
		}}
		class="flex grow"
		id={connectButtonId}
	>
		<div
			class="relative z-10 flex grow items-center rounded-lg border border-base-300 dark:border-base-400"
		>
			<button
				class="btn flex grow rounded-r-none border-transparent bg-primary/10 font-mono text-xs uppercase hover:bg-primary hover:text-primary-content"
				onclick={() => onConnect?.()}
				disabled={hasLicenseEntitlementViolations}
			>
				Connect
			</button>
			<CopyButton
				tooltipText="Copy Connect URL"
				text={connectURL}
				noButtonText
				classes={{
					button:
						'size-10 justify-center rounded-r-md border-l border-l-base-300 p-2 hover:bg-primary hover:text-primary-content dark:border-l-base-400'
				}}
			/>
		</div>
	</div>
	<button
		type="button"
		aria-label="Test vMCP"
		use:tooltip={{ text: 'Test vMCP' }}
		class="relative z-10 btn btn-square border-base-300 bg-transparent hover:bg-primary hover:text-primary-content dark:border-base-400"
		onclick={handleTest}
		disabled={hasLicenseEntitlementViolations}
	>
		<MessageCircle class="size-4" />
	</button>
</div>

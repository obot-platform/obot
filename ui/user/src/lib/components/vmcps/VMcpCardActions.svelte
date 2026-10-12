<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { m } from '$lib/i18n';
	import { MCP_CONNECTION_INVALID_LICENSE_MESSAGE } from '$lib/services/user/constants';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { profile, version, vmcpInstances } from '$lib/stores';
	import { goto } from '$lib/url';
	import { MessageCircle } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		connectURL?: string;
		connectButtonId?: string;
		connectEl?: HTMLElement;
		onConnect?: (options?: VMcpConnectOptions) => void;
		id: string;
		disabled?: boolean;
		isShared?: boolean;
		hideTest?: boolean;
	}

	let {
		connectURL,
		connectButtonId,
		connectEl = $bindable(),
		onConnect,
		id,
		disabled,
		isShared,
		hideTest = false
	}: Props = $props();

	let hasLicenseEntitlementViolations = $derived(
		(version.current.licenseEntitlementViolations || []).length > 0
	);
	let hasConfiguredInstance = $derived(
		vmcpInstances.current.items.some(
			(candidate) => candidate.vmcpID === id && candidate.userID === profile.current.id
		)
	);

	function goToTester() {
		goto(`/vmcps/${id}?view=inspector`);
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
			text: hasLicenseEntitlementViolations
				? MCP_CONNECTION_INVALID_LICENSE_MESSAGE
				: disabled
					? !isShared
						? m.vmcps_cannot_connect_personal()
						: m.vmcps_requires_access_to_connect()
					: undefined
		}}
		class="flex grow"
		id={connectButtonId}
	>
		<div
			bind:this={connectEl}
			class="relative z-10 flex grow items-center overflow-hidden rounded-lg border border-base-300 dark:border-base-400"
		>
			<button
				class={twMerge(
					'btn flex grow border-transparent bg-primary/10 font-mono text-xs uppercase not-disabled:hover:bg-primary not-disabled:hover:text-primary-content',
					connectURL ? 'rounded-r-none' : 'rounded-md'
				)}
				onclick={() => onConnect?.()}
				disabled={hasLicenseEntitlementViolations || disabled}
				aria-disabled={hasLicenseEntitlementViolations || disabled}
			>
				{m.vmcps_connect()}
			</button>
			<CopyButton
				tooltipText={m.vmcps_copy_connect_url()}
				text={connectURL}
				noButtonText
				classes={{
					button:
						'size-10 justify-center rounded-r-md border-l border-l-base-300 p-2 not-disabled:hover:bg-primary not-disabled:hover:text-primary-content dark:border-l-base-400 disabled:text-muted-content disabled:opacity-50'
				}}
				disabled={hasLicenseEntitlementViolations || disabled || !connectURL}
			/>
		</div>
	</div>
	{#if !hideTest}
		<button
			type="button"
			aria-label={m.vmcps_test_vmcp()}
			use:tooltip={{ text: m.vmcps_test_vmcp() }}
			class="relative z-10 btn btn-square border-base-300 bg-transparent hover:bg-primary hover:text-primary-content dark:border-base-400"
			onclick={handleTest}
			disabled={hasLicenseEntitlementViolations || disabled}
		>
			<MessageCircle class="size-4" />
		</button>
	{/if}
</div>

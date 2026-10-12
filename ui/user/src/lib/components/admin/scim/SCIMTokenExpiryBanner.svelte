<script lang="ts">
	import { resolve } from '$app/paths';
	import { SCIM_VIEW_PATH } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { TriangleAlert, X } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		// The display name of the auth provider whose SCIM token expires.
		providerName: string;
		expiresAt: string;
		expired: boolean;
		// Absent once the token has expired, as provisioning fails until it is rotated.
		onDismiss?: () => void;
	}

	let { providerName, expiresAt, expired, onDismiss }: Props = $props();

	let expiryDate = $derived(new Date(expiresAt).toLocaleDateString());
</script>

<div class="bg-base-100 w-full min-h-8.5">
	<div
		class={twMerge(
			'w-full py-2 px-4 flex items-center justify-center gap-2',
			expired ? 'bg-error/10 text-error' : 'bg-warning/10 text-warning'
		)}
		role={expired ? 'alert' : 'status'}
	>
		<TriangleAlert class="size-4 shrink-0" />
		<p class="text-xs font-light max-w-2xl">
			{#if expired}
				{m.identity_access_scim_token_expired_banner({ provider: providerName, date: expiryDate })}
			{:else}
				{m.identity_access_scim_token_expiring_banner({ provider: providerName, date: expiryDate })}
			{/if}
		</p>
		<a
			href={resolve(SCIM_VIEW_PATH)}
			class={twMerge('btn btn-xs shrink-0', expired ? 'btn-error' : 'btn-warning')}
		>
			{m.identity_access_scim_rotate_token_action()}
		</a>
		{#if onDismiss}
			<button
				class="btn btn-circle btn-ghost btn-xs w-fit h-fit p-0.5"
				onclick={onDismiss}
				type="button"
				aria-label={m.identity_access_scim_dismiss_token_banner()}
			>
				<X class="size-3" />
			</button>
		{/if}
	</div>
</div>

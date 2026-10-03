<script lang="ts">
	import { resolve } from '$app/paths';
	import { SCIM_VIEW_PATH } from '$lib/constants';
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
				{`The SCIM token for ${providerName} expired on ${expiryDate}, so provisioning from ${providerName} fails. Rotate the token, and update it in ${providerName}.`}
			{:else}
				{`The SCIM token for ${providerName} expires on ${expiryDate}. Rotate it before then, and update it in ${providerName}.`}
			{/if}
		</p>
		<a
			href={resolve(SCIM_VIEW_PATH)}
			class={twMerge('btn btn-xs shrink-0', expired ? 'btn-error' : 'btn-warning')}
		>
			Rotate SCIM token
		</a>
		{#if onDismiss}
			<button
				class="btn btn-circle btn-ghost btn-xs w-fit h-fit p-0.5"
				onclick={onDismiss}
				type="button"
				aria-label="Dismiss SCIM token banner"
			>
				<X class="size-3" />
			</button>
		{/if}
	</div>
</div>

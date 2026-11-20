<script lang="ts" module>
	import type { APIKey } from '$lib/services';

	export function isUsableLlmApiKey(key: APIKey): boolean {
		if (!key.canAccessLLMProxy) return false;
		if (!key.expiresAt) return true;
		const expires = new Date(key.expiresAt).getTime();
		return !Number.isNaN(expires) && expires > Date.now();
	}

	export function hasUsableLlmApiKey(keys: readonly APIKey[]): boolean {
		return keys.some(isUsableLlmApiKey);
	}
</script>

<script lang="ts">
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import type { APIKey as APIKeyType } from '$lib/services';
	import { resolve } from '$lib/url';
	import { ChevronDown } from '@lucide/svelte';

	interface Props {
		apiKeys?: readonly APIKeyType[];
		onGenerate: () => void | Promise<unknown>;
		hasGeneratedKey?: boolean;
		generating?: boolean;
		block?: boolean;
		disablePortal?: boolean;
	}

	let {
		apiKeys = [],
		onGenerate,
		hasGeneratedKey = false,
		generating = false,
		disablePortal = false
	}: Props = $props();

	let hasExistingKeys = $derived(hasGeneratedKey || hasUsableLlmApiKey(apiKeys));
</script>

{#if hasExistingKeys}
	<div class="flex shrink-0 w-full @lg:w-fit">
		<a
			class="btn btn-secondary grow btn-soft btn-sm text-base-content border-base-400 shrink-0 text-nowrap rounded-r-none!"
			href={resolve('/identity-access?view=agents')}
		>
			{m.models_gateway_manage_api_keys()}
		</a>
		<DotDotDot
			class="btn-secondary btn-sm text-base-content btn-soft border-base-400 rounded-l-none! border-l-0! pl-2! pr-2.5! btn-block w-fit"
			placement="bottom-end"
			classes={{
				popover: 'w-44.5'
			}}
			{disablePortal}
		>
			{#snippet icon()}
				<ChevronDown class="size-4" />
			{/snippet}
			{#snippet children({ toggle })}
				<button
					type="button"
					class="menu-button min-h-fit text-xs"
					disabled={generating}
					onclick={async () => {
						await onGenerate();
						toggle(false);
					}}
				>
					{#if generating}
						<Loading class="size-4" />
					{:else}
						{m.models_gateway_generate_new_key()}
					{/if}
				</button>
			{/snippet}
		</DotDotDot>
	</div>
{:else}
	<button
		type="button"
		class="btn btn-primary btn-sm shrink-0 text-nowrap w-full @lg:w-fit"
		disabled={generating}
		onclick={() => void onGenerate()}
	>
		{#if generating}
			<Loading class="size-4 text-primary-content" />
		{:else}
			{m.models_gateway_generate_api_key()}
		{/if}
	</button>
{/if}

<script lang="ts">
	import CopyField from '$lib/components/CopyField.svelte';
	import { m } from '$lib/i18n';
	import { KeyRound, TriangleAlert } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		keyValue: string;
		id: string;
		class?: string;
	}

	let { keyValue, id, class: klass = '' }: Props = $props();
</script>

<div class={twMerge('flex flex-col gap-6', klass)}>
	<div class="notification-alert">
		<div class="flex items-start gap-3">
			<TriangleAlert class="size-5 shrink-0" />
			<div class="flex flex-col gap-1">
				<p class="text-sm font-medium">{m.identity_access_agents_save_this_key_now()}</p>
				<p class="text-xs font-light">
					{m.identity_access_agents_api_key_only_time()}
				</p>
			</div>
		</div>
	</div>

	<label class="flex flex-col gap-2" for={id}>
		<span class="text-sm font-medium">{m.identity_access_agents_your_api_key()}</span>
		<CopyField value={keyValue} {id}>
			{#snippet preContent()}
				<KeyRound class="text-muted-content size-4 shrink-0" />
			{/snippet}
		</CopyField>
	</label>
</div>

<script lang="ts">
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import { TriangleAlert, KeyRound, ExternalLink } from '@lucide/svelte';

	interface Props {
		keyValue?: string;
		onClose: () => void;
	}

	let { keyValue, onClose }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	$effect(() => {
		if (keyValue) {
			dialog?.open();
		}
	});

	function handleClose() {
		onClose();
		dialog?.close();
	}
</script>

{#if keyValue}
	<ResponsiveDialog
		bind:this={dialog}
		onClose={handleClose}
		title={m.identity_access_agents_api_key_created()}
		class="w-full max-w-lg"
		disableClickOutside
	>
		<div class="flex flex-col gap-6">
			<div class="notification-alert">
				<div class="flex items-start gap-3">
					<TriangleAlert class="size-5 shrink-0" />
					<div class="flex flex-col gap-1">
						<p class="text-sm font-medium">{m.identity_access_agents_save_this_key_now()}</p>
						<p class="text-xs">
							{m.identity_access_agents_api_key_only_time()}
						</p>
					</div>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium">{m.identity_access_agents_your_api_key()}</p>
				<CopyField value={keyValue} id="agent-auth-scope-key">
					{#snippet preContent()}
						<KeyRound class="text-muted-content size-4 shrink-0" />
					{/snippet}
				</CopyField>
			</div>

			<p class="text-muted text-sm">
				{m.identity_access_agents_learn_api_key_docs_prefix()}
				<a
					href="https://docs.obot.ai/functionality/api-keys/#using-an-api-key"
					target="_blank"
					rel="noopener noreferrer"
					class="text-link inline-flex items-center gap-1"
				>
					{m.identity_access_agents_learn_api_key_docs_link()}
					<ExternalLink class="size-3" />
				</a>{m.identity_access_agents_learn_api_key_docs_suffix()}
			</p>
		</div>

		<div class="mt-6 flex justify-end">
			<button class="btn btn-primary" onclick={handleClose}>
				{m.identity_access_agents_ive_saved_my_key()}
			</button>
		</div>
	</ResponsiveDialog>
{/if}

<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import ApiKeyRevealNotice from '$lib/components/agent-auth-scope/ApiKeyRevealNotice.svelte';
	import { m } from '$lib/i18n';
	import { ExternalLink } from '@lucide/svelte';

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
			<ApiKeyRevealNotice keyValue={keyValue ?? ''} id="agent-auth-scope-key" />

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

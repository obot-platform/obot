<script lang="ts">
	import { resolve } from '$app/paths';
	import { SCIM_VIEW_PATH } from '$lib/constants';
	import { m } from '$lib/i18n';

	// The banner cannot be dismissed: until SCIM is enforced, users that the identity provider has not
	// provisioned can still sign in.
	interface Props {
		// The display name of the auth provider whose SCIM setup is not finished.
		providerName: string;
	}

	let { providerName }: Props = $props();
</script>

<div class="bg-base-100 w-full min-h-8.5">
	<div
		class="bg-primary/10 text-primary w-full py-2 px-4 flex items-center justify-center gap-2"
		role="status"
	>
		<p class="text-xs font-light max-w-2xl">
			{m.identity_access_scim_setup_banner({ provider: providerName })}
		</p>
		<a href={resolve(SCIM_VIEW_PATH)} class="btn btn-xs btn-primary shrink-0">
			{m.identity_access_scim_continue_setup()}
		</a>
	</div>
</div>

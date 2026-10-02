<script lang="ts">
	import { resolve } from '$app/paths';
	import { SCIM_VIEW_PATH } from '$lib/constants';

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
			{providerName} provisions users and groups through SCIM, and SCIM is not enforced yet. Continue
			setup on Identity & Access → Auth Providers → SCIM: create the SCIM app in {providerName} with the
			base URL and token, assign users, push groups, and enforce SCIM.
		</p>
		<a href={resolve(SCIM_VIEW_PATH)} class="btn btn-xs btn-primary shrink-0">
			Continue SCIM setup
		</a>
	</div>
</div>

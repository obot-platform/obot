<script lang="ts">
	import RedirectLayout from '$lib/components/RedirectLayout.svelte';
	import { m } from '$lib/i18n';
	import { MESSAGE_POLICIES_REDIRECT_DESTINATIONS } from '../constants';
</script>

<RedirectLayout
	title={m.admin_routes_message_policies()}
	destinations={MESSAGE_POLICIES_REDIRECT_DESTINATIONS}
/>

<script lang="ts">
	import RedirectLayout from '$lib/components/RedirectLayout.svelte';
	import { MESSAGE_POLICIES_REDIRECT_DESTINATIONS } from '../constants';
</script>

<RedirectLayout title="Message Policies" destinations={MESSAGE_POLICIES_REDIRECT_DESTINATIONS} />

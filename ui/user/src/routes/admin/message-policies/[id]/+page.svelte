<script lang="ts">
	import RedirectLayout from '$lib/components/RedirectLayout.svelte';
	import { m } from '$lib/i18n';
	import { MESSAGE_POLICIES_REDIRECT_DESTINATIONS } from '../constants';
</script>

<RedirectLayout
	title={m.ai_judge_message_policies()}
	destinations={MESSAGE_POLICIES_REDIRECT_DESTINATIONS}
/>

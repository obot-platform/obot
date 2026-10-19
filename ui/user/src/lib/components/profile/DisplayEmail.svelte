<script lang="ts">
	import { formatUserEmail } from '$lib/emailDisplay';

	let { user }: { user: { email?: string; emailDisplayDomain?: string } } = $props();
</script>

<span>{formatUserEmail(user)}</span>

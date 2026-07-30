<script lang="ts">
	import { version } from '$lib/stores';
	import { ShieldAlert } from '@lucide/svelte';

	let hasUserLimitViolation = $derived(
		version.current.licenseEntitlementViolations?.some(
			(violation) => violation.type === 'userLimit'
		) ?? false
	);

	let userLimitText = $derived(
		version.current.userLimit && version.current.userCount
			? `(${version.current.userCount}/${version.current.userLimit})`
			: ''
	);
</script>

<div class="notification-alert p-3 text-sm font-light flex items-center gap-2">
	<ShieldAlert class="size-6" />
	<div>
		You're {hasUserLimitViolation ? 'at' : 'almost at'} the user limit. {userLimitText}
		<a
			href="https://obot.ai/contact-us/"
			class="text-link"
			target="_blank"
			rel="noopener noreferrer"
		>
			Contact us to upgrade to Obot Enterprise</a
		>
	</div>
</div>

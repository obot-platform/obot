<script lang="ts">
	import { version } from '$lib/stores';

	let hasUserLimitViolation = $derived(
		version.current.licenseEntitlementViolations?.some(
			(violation) => violation.type === 'userLimit'
		) ?? false
	);

	let userLimitText = $derived(
		version.current.userLimit && version.current.userCount
			? `(${version.current.userCount} / ${version.current.userLimit})`
			: ''
	);
</script>

<div class="notification-alert p-3 text-sm font-light">
	You're {hasUserLimitViolation ? 'at' : 'almost at'} the user limit. {userLimitText}
	<a href="https://obot.ai/contact-us/" class="text-link"
		>Contact us to upgrade to Enterprise Edition</a
	>
</div>

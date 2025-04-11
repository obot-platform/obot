<script lang="ts">
	import { page } from '$app/state';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';

	const errorTitles = {
		403: 'Access Denied',
		404: 'Page Not Found',
		500: 'Internal Server Error'
	};

	const defaultMessage = {
		403: 'You are not allowed to access this page.',
		404: 'It looks like the page you are trying to access does not exist.',
		500: 'An error occurred while loading the page.'
	};

	const title = errorTitles[page.status as keyof typeof errorTitles] || 'Error';
	const message =
		defaultMessage[page.status as keyof typeof defaultMessage] || 'Please try again later.';
</script>

<div class="flex h-screen w-full flex-col items-center justify-center gap-4">
	<div class="flex flex-col items-center justify-center">
		<div>
			{#if page.status === 404}
				<img alt="Not Found Obot" src="/user/images/obot-404.webp" class="max-w-xs md:max-w-sm" />
			{:else if page.status === 403}
				<img alt="Forbidden Obot" src="/user/images/obot-403.webp" class="max-w-xs md:max-w-sm" />
			{:else}
				<img
					alt="Internal Server Error Obot"
					src="/user/images/obot-500.webp"
					class="max-w-xs md:max-w-sm"
				/>
			{/if}
		</div>
		<h1 class="text-xl font-semibold md:text-2xl">{title}</h1>
	</div>
	<p class="text-gray px-4">{message}</p>

	{#if page.error}
		<div class="mb-2 w-full max-w-xl overflow-hidden rounded-md px-4">
			<CollapsePane
				header="More Details"
				classes={{
					header: 'bg-surface2 justify-between',
					content: 'bg-surface1'
				}}
			>
				<div class="">{page.error.message}</div>
			</CollapsePane>
		</div>
	{/if}

	<a href="/home" class="button-primary"> Go Home </a>
</div>

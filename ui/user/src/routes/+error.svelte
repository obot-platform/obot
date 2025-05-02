<script lang="ts">
	import { page } from '$app/state';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';

	const errorTitles = {
		401: 'Unauthorized',
		403: 'Access Denied',
		404: 'Page Not Found',
		500: 'Internal Server Error'
	};

	const defaultMessage = {
		401: 'You are not logged in.',
		403: 'You are not allowed to access this page.',
		404: 'It looks like the page you are trying to access does not exist.',
		500: 'An error occurred while loading the page.'
	};

	const title = errorTitles[page.status as keyof typeof errorTitles] || 'Error';
	const message =
		defaultMessage[page.status as keyof typeof defaultMessage] || 'Please try again later.';
</script>

<div class="flex h-screen w-full flex-col items-center justify-center gap-4">
	<div class="flex items-end justify-end gap-8">
		<div>
			<img
				alt="Grumpy obot"
				src="/user/images/obot-icon-grumpy-blue.svg"
				class="h-[200px] w-[200px]"
			/>
		</div>
		<div
			class="speech-bubble bg-surface2 after:border-r-surface2 relative m-4 flex flex-col items-center justify-center rounded-md
    p-4 after:absolute after:top-[50%] after:left-0 after:mt-[-20px] after:ml-[-40px]
    after:h-0 after:w-0 after:border-[40px] after:border-b-0
    after:border-l-0 after:border-transparent after:content-['']"
		>
			<div class="text-8xl font-bold">{page.status}</div>
			<h1 class="text-xl font-semibold">{title}</h1>
		</div>
	</div>
	<p class="text-gray">{message}</p>

	{#if page.error}
		<div class="mb-2 w-full max-w-xl overflow-hidden rounded-md">
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

	<a href="/agents" class="button-primary"> Go Home </a>
</div>

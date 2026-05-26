<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Logo from '$lib/components/Logo.svelte';

	const errorTitles = {
		400: 'Bad Request',
		401: 'Unauthorized',
		403: 'Access Denied',
		404: 'Page Not Found',
		422: 'Unprocessable Entity',
		429: 'Too Many Requests',
		500: 'Internal Server Error'
	};

	const defaultErrorCodeMessage = {
		401: 'You are not logged in.',
		403: 'You are not allowed to access this page.',
		404: 'It looks like the page you are trying to access does not exist.',
		500: 'An error occurred while loading the page.'
	};

	const defaultMessage = defaultErrorCodeMessage[500];

	const title = errorTitles[page.status as keyof typeof errorTitles] || 'Error';
	const message =
		defaultErrorCodeMessage[page.status as keyof typeof defaultErrorCodeMessage] || defaultMessage;
</script>

<div class="flex h-dvh w-full flex-col items-center justify-center gap-4">
	<div class="flex items-end justify-end gap-8">
		<div>
			<Logo variant="error" class="h-[200px] w-[200px]" />
		</div>
		<div
			class="speech-bubble bg-base-300 after:border-r-base-300 relative m-4 flex flex-col items-center justify-center rounded-md
    p-4 after:absolute after:top-[50%] after:left-0 after:mt-[-20px] after:ml-[-40px]
    after:h-0 after:w-0 after:border-40 after:border-b-0
    after:border-l-0 after:border-transparent after:content-['']"
		>
			<div class="text-8xl font-bold">{page.status}</div>
			<h1 class="text-xl font-semibold">{title}</h1>
		</div>
	</div>
	<p class="text-gray">{message}</p>

	{#if page.error?.message}
		<details class="collapse bg-base-300 collapse-arrow border max-w-xl mb-2 w-full">
			<summary class="collapse-title font-semibold text-base">Error Details</summary>
			<div class="collapse-content p-0 text-sm bg-base-200 space-y-3">
				<div class="p-4 overflow-y-auto default-scrollbar-thin max-h-64">
					{#if page.error.message}
						<p class="wrap-break-word">{page.error.message}</p>
					{/if}
				</div>
			</div>
		</details>
	{/if}

	<a href={resolve('/')} class="btn btn-primary"> Go Home </a>
</div>

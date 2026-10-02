<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Logo from '$lib/components/Logo.svelte';
	import { m } from '$lib/i18n';

	const errorTitles = {
		400: m.error_page_400,
		401: m.error_page_401,
		403: m.error_page_403,
		404: m.error_page_404,
		422: m.error_page_422,
		429: m.error_page_429,
		500: m.error_page_500
	};

	const defaultErrorCodeMessage = {
		401: m.error_page_401_message,
		403: m.error_page_403_message,
		404: m.error_page_404_message,
		500: m.error_page_500_message
	};

	const defaultMessage = defaultErrorCodeMessage[500];

	const title = (errorTitles[page.status as keyof typeof errorTitles] ?? m.common_error)();
	const message = (
		defaultErrorCodeMessage[page.status as keyof typeof defaultErrorCodeMessage] ?? defaultMessage
	)();
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
			<summary class="collapse-title font-semibold text-base">{m.error_page_details()}</summary>
			<div class="collapse-content p-0 text-sm bg-base-200 space-y-3">
				<div class="p-4 overflow-y-auto default-scrollbar-thin max-h-64">
					{#if page.error.message}
						<p class="wrap-break-word">{page.error.message}</p>
					{/if}
				</div>
			</div>
		</details>
	{/if}

	<a href={resolve('/')} class="btn btn-primary">{m.error_page_go_home()}</a>
</div>

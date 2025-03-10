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
	<div class="flex items-end justify-end gap-8">
		<div>
			<img
				alt="Grumpy obot"
				src="/user/images/obot-icon-grumpy-blue.svg"
				class="h-[200px] w-[200px]"
			/>
		</div>
		<div
			class="speech-bubble relative m-4 flex flex-col items-center justify-center rounded-md bg-surface2 p-4
    after:absolute after:left-0 after:top-[50%] after:ml-[-40px] after:mt-[-20px] after:h-0
    after:w-0 after:border-[40px] after:border-b-0 after:border-l-0
    after:border-transparent after:border-r-surface2 after:content-['']"
		>
			<div class="text-8xl font-bold">{page.status}</div>
			<h1 class="text-xl font-semibold">{title}</h1>
		</div>
	</div>
	<p class="text-gray">{message}</p>

	{#if page.error}
		<div class="w-full max-w-xl overflow-hidden rounded-md mb-2">
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

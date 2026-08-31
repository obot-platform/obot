<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import Select from '$lib/components/Select.svelte';
	import CustomConfigurationForm from '$lib/components/mcp/CustomConfigurationForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { formatTimeAgo } from '$lib/time';
	import { MOCK_CONNECTOR_TABLE_DATA, type BrandingMockConnectorRow } from './constants.js';
	import { CircleAlert, HouseIcon, Info, X } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const duration = PAGE_TRANSITION_DURATION;
</script>

<div class="relative mb-8 h-full min-w-0 w-full @container" transition:fade={{ duration }}>
	<div>
		<div class="notification-info p-3 text-sm font-light">
			<div class="flex items-center gap-3">
				<Info class="size-6 shrink-0" />
				<div class="flex flex-col gap-1">
					<p class="font-semibold">Example Components</p>
					<p>
						Below are some example components used in the application for easy previewing. This
						itself is a commonly used notification that is displayed to provide information to the
						user in a detail view.
					</p>
				</div>
			</div>
		</div>
	</div>
	<div class="grid grid-cols-12 gap-4 mt-8">
		<div class="relative h-72 col-span-12 @min-[768px]:col-span-6">
			<div
				class="absolute top-1/2 left-1/2 flex w-md -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-4"
			>
				<Logo class="h-16" />
				<h1 class="text-2xl font-semibold">Welcome to Obot</h1>
				<p class="text-md text-muted-content mb-1 text-center font-light">
					Log in or create your account to continue
				</p>

				<div
					class="dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-sm flex-col gap-4 rounded-xl border border-transparent p-4 shadow-sm"
				>
					<button class="btn btn-secondary w-full">
						<img
							class="h-6 w-6 rounded-full bg-base-100 p-1 dark:bg-gray-600"
							src="/user/images/github-mark/github-mark.svg"
							alt="Github"
						/>
						<span class="text-center text-sm font-light">Continue with Github</span>
					</button>
				</div>
			</div>
		</div>
		<div class="flex justify-center items-center col-span-12 @min-[768px]:col-span-6">
			<div class="dialog-container max-w-md">
				<div class="dialog-title p-4 pb-0">
					Confirm Action
					<button type="button">
						<X class="size-5" />
					</button>
				</div>
				<div class="flex flex-col items-center justify-center gap-2 p-4 pt-0">
					<div class="rounded-full p-2 bg-primary/10">
						<CircleAlert class="size-8 text-primary" />
					</div>
					<p class="text-center text-base font-medium">
						Are you sure you want to confirm this action?
					</p>

					<div class="mb-4 self-center text-center font-light">
						<p>
							This is an example of a confirmation dialog. It can be used to confirm any action that
							is irreversible or information that needs to be conveyed before submission.
						</p>
					</div>

					<div
						class="flex w-full flex-col items-center justify-center gap-2 @min-[768px]:flex-col @min-[768px]:justify-end"
					>
						<button type="button" class="flex w-full justify-center p-3 btn btn-primary">
							Confirm
						</button>
						<button type="button" class="btn btn-secondary w-full justify-center">Cancel</button>
					</div>
				</div>
			</div>
		</div>
	</div>
	<div class="flex gap-4 items-center flex-wrap mt-8">
		<div class="flex gap-4 grow flex-wrap">
			<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
				<button class="btn btn-circle btn-primary"><HouseIcon /></button>
				<button class="btn btn-primary">Confirm</button>
			</div>
			<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
				<button class="btn btn-circle btn-secondary"><HouseIcon /></button>
				<button class="btn btn-secondary">Confirm</button>
			</div>
			<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
				<button class="btn btn-circle btn-success"><HouseIcon /></button>
				<button class="btn btn-success">Confirm</button>
			</div>
			<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
				<button class="btn btn-circle btn-warning"><HouseIcon /></button>
				<button class="btn btn-warning">Confirm</button>
			</div>
			<div class="bg-base-100 dark:bg-base-200 rounded-md p-3 flex gap-4">
				<button class="btn btn-circle btn-error"><HouseIcon /></button>
				<button class="btn btn-error">Confirm</button>
			</div>
		</div>
	</div>
	<div class="w-full mt-8">
		<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
			<div class="flex">
				<button class="page-tab w-1/2 max-w-1/2 page-tab-active"> Servers </button>
				<button class="page-tab w-1/2 max-w-1/2"> Users </button>
			</div>
			<Table
				data={MOCK_CONNECTOR_TABLE_DATA}
				fields={['name', 'status', 'created']}
				filterable={['name', 'status']}
				sortable={['name', 'created', 'status']}
			>
				{#snippet onRenderColumn(field: string, row: BrandingMockConnectorRow)}
					{#if field === 'name'}
						<span class="flex items-center gap-2">
							<i class={twMerge('devicon', row.devicon)}></i>
							{row.name}
						</span>
					{/if}
					{#if field === 'status'}
						{#if row.status === 'Connected'}
							<div class="pill-primary bg-primary">{row.status}</div>
						{:else}
							<div class="text-xs font-light">{row.status}</div>
						{/if}
					{/if}
					{#if field === 'created'}
						{formatTimeAgo(row.created).relativeTime}
					{/if}
				{/snippet}
			</Table>
		</div>
	</div>

	<div class="w-full paper my-8">
		<h4 class="text-lg font-semibold">Custom Form</h4>
		<div class="flex flex-col gap-1">
			<label for="description" class="text-sm font-light">Description</label>
			<input class="text-input-filled" placeholder="Write a description..." />
		</div>
		<div class="flex gap-4 items-center justify-between">
			<p class="text-sm font-light">Example Selector</p>
			<div class="flex grow">
				<Select
					class="bg-base-200 dark:bg-base-100 dark:border-base-400 border border-transparent shadow-inner"
					classes={{
						root: 'flex grow'
					}}
					selected="a"
					options={[
						{ label: 'Option 1', id: 'a' },
						{ label: 'Option 2', id: 'b' },
						{ label: 'Option 3', id: 'c' },
						{ label: 'Option 4', id: 'd' }
					]}
				/>
			</div>
		</div>
		<div class="flex justify-end">
			<label for="toggle" class="label text-sm">
				Toggle
				<input id="toggle" type="checkbox" checked={true} class="toggle" />
			</label>
		</div>
	</div>

	<CustomConfigurationForm
		config={[
			{
				key: 'Example Key',
				value: 'Example Value',
				description: 'Example Description',
				name: 'Example Name',
				required: true,
				sensitive: false
			}
		]}
	/>
</div>

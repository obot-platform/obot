<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import {
		AdminService,
		type ProjectThread,
		type Project,
		type OrgUser,
		type Task,
		Group
	} from '$lib/services';
	import { Eye, LoaderCircle, MessageCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { replaceState } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';
	import { page } from '$app/state';
	import { debounce } from 'es-toolkit';
	import { profile } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';
	import { openUrl } from '$lib/utils';
	import { clearUrlParams, setUrlParams } from '$lib/url';

	let threads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let tasks = $state<Task[]>([]);

	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));
	let taskMap = $derived(new Map(tasks.map((t) => [t.id, t])));

	let query = $state(page.url.searchParams.get('query') || '');
	let urlFilters = $derived.by<Record<string, (string | number)[]>>(() => {
		return page.url.searchParams
			.entries()
			.filter((entry) => entry[0] !== 'query')
			.reduce(
				(acc, [key, value]) => {
					acc[key] = value.split(',');
					return acc;
				},
				{} as Record<string, (string | number)[]>
			);
	});

	let loading = $state(true);
	let filteredThreads = $derived(threads.filter((thread) => !thread.project && !thread.systemTask));
	let tableData = $derived(
		filteredThreads.map((thread) => {
			return {
				...thread,
				projectName: projectMap.get(thread.projectID || '') || thread.projectID,
				userName: userMap.get(thread.userID || '')?.displayName || '-',
				userEmail: userMap.get(thread.userID || '')?.email || '-',
				task: taskMap.get(thread.taskID || '')?.name || '-'
			};
		})
	);

	let convertedUrlFilters = $derived.by(() => {
		const { task, ...rest } = urlFilters;
		// Convert task to taskID for filtering
		if (task) {
			return {
				...rest,
				taskID: task
			};
		}
		return rest;
	});

	const updateQuery = debounce((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		// Update the query search param without cause app to react
		// Prevent losing focus from the input
		// history.replaceState(null, '', page.url);
		replaceState(page.url, { query });
	}, 100);

	let isAuditor = $derived(profile.current.groups.includes(Group.AUDITOR));

	onMount(() => {
		loadThreads();
	});

	async function loadThreads() {
		loading = true;
		try {
			// Load threads, projects, and users in parallel with individual error handling
			const threadsPromise = AdminService.listThreads().catch((err) => {
				console.error('Failed to load threads:', err);
				return [];
			});

			const projectsPromise = AdminService.listProjects().catch((err) => {
				console.error('Failed to load projects:', err);
				return [];
			});

			const usersPromise = AdminService.listUsers().catch((err) => {
				console.error('Failed to load users:', err);
				return [];
			});

			const tasksPromise = AdminService.listTasks().catch((err) => {
				console.error('Failed to load tasks:', err);
				return [];
			});

			// Add timeout to prevent hanging
			const timeoutPromise = new Promise<never>((_, reject) => {
				setTimeout(() => reject(new Error('Request timeout')), 10000);
			});

			const [threadsData, projectsData, usersData, tasksData] = await Promise.race([
				Promise.all([threadsPromise, projectsPromise, usersPromise, tasksPromise]),
				timeoutPromise
			]);

			// threads = threadsData;
			projects = projectsData;
			users = usersData;
			// Filter to only include task runs
			threads = threadsData.filter((thread) => !!thread.taskRunID);
			tasks = tasksData;
		} catch (error) {
			console.error('Failed to load data:', error);
			// Set empty arrays as fallback
			threads = [];
			projects = [];
			users = [];
			tasks = [];
		} finally {
			loading = false;
		}
	}

	function handleViewThread(thread: ProjectThread, isCtrlClick: boolean) {
		const url = `/admin/task-runs/${thread.id}`;
		openUrl(url, isCtrlClick);
	}
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8 pb-8">
			<h1 class="text-2xl font-semibold">Task Runs</h1>

			<div class="flex flex-col gap-2">
				<Search
					value={query}
					class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
					onChange={updateQuery}
					placeholder="Search threads..."
				/>

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="size-8 animate-spin text-blue-600" />
					</div>
				{:else if filteredThreads.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="size-24 text-gray-200 dark:text-gray-700" />
						<h3 class="mt-4 text-lg font-semibold text-gray-400 dark:text-gray-600">
							No task runs available
						</h3>
						<p class="mt-2 text-sm font-light text-gray-400 dark:text-gray-600">
							Task runs will appear here once they are created.
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['name', 'userName', 'userEmail', 'task', 'projectName', 'created']}
						filterable={['name', 'userName', 'userEmail', 'task', 'projectName']}
						onFilter={setUrlParams}
						filters={convertedUrlFilters}
						onClearAllFilters={clearUrlParams}
						onClickRow={isAuditor ? handleViewThread : undefined}
						headers={[
							{
								title: 'User Name',
								property: 'userName'
							},
							{
								title: 'User Email',
								property: 'userEmail'
							},
							{
								title: 'Project',
								property: 'projectName'
							}
						]}
						headerClasses={[
							{
								property: 'name',
								class: 'w-4/12 min-w-sm'
							}
						]}
						sortable={['name', 'userName', 'userEmail', 'projectName', 'created', 'task']}
						initSort={{ property: 'created', order: 'desc' }}
					>
						{#snippet actions()}
							<button
								class={twMerge(
									'icon-button',
									isAuditor && 'hover:text-blue-500',
									!isAuditor && 'opacity-50 hover:bg-transparent dark:hover:bg-transparent'
								)}
								title="View Thread"
								use:tooltip={{
									text: isAuditor
										? 'View Task Run'
										: 'To view details, auditing permissions are required.'
								}}
								disabled={!isAuditor}
							>
								<Eye class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, thread)}
							{#if property === 'name'}
								<span>{thread.name || 'Unnamed Task Run'}</span>
							{:else if property === 'created'}
								<span class="text-sm text-gray-600 dark:text-gray-400">
									{formatTimeAgo(thread.created).relativeTime}
								</span>
							{:else}
								{thread[property as keyof typeof thread]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Admin - Task Runs</title>
</svelte:head>

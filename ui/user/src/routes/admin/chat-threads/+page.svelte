<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import Select from '$lib/components/Select.svelte';
	import { AdminService, type ProjectThread, type Project, type OrgUser } from '$lib/services';
	import { Eye, LoaderCircle, MessageCircle, ListFilter } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly, slide } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';

	let threads = $state<ProjectThread[]>([]);
	let filteredThreads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	// Get unique options from thread data for Select components
	let usernameOptions = $derived.by(() => {
		const usernames = new Set<string>();
		threads.forEach((thread) => {
			const user = userMap.get(thread.userID || '');
			if (user?.displayName) {
				usernames.add(user.displayName);
			}
		});
		return Array.from(usernames)
			.sort()
			.map((username) => ({ id: username, label: username }));
	});

	let emailOptions = $derived.by(() => {
		const emails = new Set<string>();
		threads.forEach((thread) => {
			const user = userMap.get(thread.userID || '');
			if (user?.email) {
				emails.add(user.email);
			}
		});
		return Array.from(emails)
			.sort()
			.map((email) => ({ id: email, label: email }));
	});

	let projectOptions = $derived.by(() => {
		const projectNames = new Set<string>();
		threads.forEach((thread) => {
			const projectName = projectMap.get(thread.projectID || '') || thread.projectID;
			if (projectName) {
				projectNames.add(projectName);
			}
		});
		return Array.from(projectNames)
			.sort()
			.map((projectName) => ({ id: projectName, label: projectName }));
	});
	let loading = $state(true);
	let searchTerm = $state('');
	let showFilters = $state(false);
	let usernameFilter = $state<string>('');
	let emailFilter = $state<string>('');
	let projectFilter = $state<string>('');
	let sortField = $state<'created' | 'name' | 'userID' | 'projectID' | 'userName' | 'userEmail'>(
		'created'
	);
	let sortDirection = $state<'asc' | 'desc'>('desc');
	let tableData = $derived(
		filteredThreads.map((thread) => {
			return {
				...thread,
				projectName: projectMap.get(thread.projectID || '') || thread.projectID,
				userName: userMap.get(thread.userID || '')?.displayName || '-',
				userEmail: userMap.get(thread.userID || '')?.email || '-'
			};
		})
	);

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

			// Add timeout to prevent hanging
			const timeoutPromise = new Promise<never>((_, reject) => {
				setTimeout(() => reject(new Error('Request timeout')), 10000);
			});

			const [threadsData, projectsData, usersData] = await Promise.race([
				Promise.all([threadsPromise, projectsPromise, usersPromise]),
				timeoutPromise
			]);

			threads = threadsData;
			projects = projectsData;
			users = usersData;
			// Filter to only include project threads (project: true) and exclude system tasks
			filteredThreads = threads.filter((thread) => thread.project && !thread.systemTask);
		} catch (error) {
			console.error('Failed to load data:', error);
			// Set empty arrays as fallback
			threads = [];
			projects = [];
			users = [];
			filteredThreads = [];
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		// First filter to only include project threads and exclude system tasks
		let filtered = threads.filter((thread) => !thread.project && !thread.systemTask);

		// Then apply search filter
		if (searchTerm.trim() !== '') {
			const term = searchTerm.toLowerCase();
			filtered = filtered.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return (
					thread.name?.toLowerCase().includes(term) ||
					thread.id.toLowerCase().includes(term) ||
					thread.userID?.toLowerCase().includes(term) ||
					thread.projectID?.toLowerCase().includes(term) ||
					user?.displayName?.toLowerCase().includes(term) ||
					user?.email?.toLowerCase().includes(term) ||
					projectMap
						.get(thread.projectID || '')
						?.toLowerCase()
						.includes(term)
				);
			});
		}

		// Apply specific filters
		if (usernameFilter.trim() !== '') {
			const usernameTerm = usernameFilter.toLowerCase();
			filtered = filtered.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return user?.displayName?.toLowerCase().includes(usernameTerm);
			});
		}

		if (emailFilter.trim() !== '') {
			const emailTerm = emailFilter.toLowerCase();
			filtered = filtered.filter((thread) => {
				const user = userMap.get(thread.userID || '');
				return user?.email?.toLowerCase().includes(emailTerm);
			});
		}

		if (projectFilter.trim() !== '') {
			const projectTerm = projectFilter.toLowerCase();
			filtered = filtered.filter((thread) => {
				const projectName = projectMap.get(thread.projectID || '') || thread.projectID;
				return projectName?.toLowerCase().includes(projectTerm);
			});
		}

		// Apply sorting
		filtered.sort((a, b) => {
			let aValue: string | number;
			let bValue: string | number;

			// Handle special cases for sorting
			if (sortField === 'created') {
				aValue = new Date(a.created).getTime();
				bValue = new Date(b.created).getTime();
			} else if (sortField === 'name') {
				aValue = (a.name || '').toLowerCase();
				bValue = (b.name || '').toLowerCase();
			} else if (sortField === 'userName') {
				const userA = userMap.get(a.userID || '');
				const userB = userMap.get(b.userID || '');
				aValue = (userA?.displayName || '').toLowerCase();
				bValue = (userB?.displayName || '').toLowerCase();
			} else if (sortField === 'userEmail') {
				const userA = userMap.get(a.userID || '');
				const userB = userMap.get(b.userID || '');
				aValue = (userA?.email || '').toLowerCase();
				bValue = (userB?.email || '').toLowerCase();
			} else {
				aValue = ((a[sortField] as string) || '').toLowerCase();
				bValue = ((b[sortField] as string) || '').toLowerCase();
			}

			if (sortDirection === 'asc') {
				return aValue > bValue ? 1 : aValue < bValue ? -1 : 0;
			} else {
				return aValue < bValue ? 1 : aValue > bValue ? -1 : 0;
			}
		});

		filteredThreads = filtered;
	});

	function handleViewThread(thread: ProjectThread) {
		// Navigate to thread view
		goto(`/admin/chat-threads/${thread.id}`);
	}

	function formatThreadName(thread: ProjectThread) {
		return thread.name || 'Unnamed Thread';
	}

	function handleUsernameSelect(option: { id: string; label: string }) {
		usernameFilter = option.id;
	}

	function handleEmailSelect(option: { id: string; label: string }) {
		emailFilter = option.id;
	}

	function handleProjectSelect(option: { id: string; label: string }) {
		projectFilter = option.id;
	}
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8 pb-8">
			<h1 class="text-2xl font-semibold">Chat Threads</h1>

			<div class="flex flex-col gap-2">
				<div class="flex items-center gap-4">
					<Search
						class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
						onChange={(val) => (searchTerm = val)}
						placeholder="Search threads..."
					/>
					<button onclick={() => (showFilters = !showFilters)} class="icon-button flex-shrink-0">
						<ListFilter class="size-6 flex-shrink-0" />
					</button>
				</div>

				{#if showFilters}
					<div
						in:slide={{ axis: 'y' }}
						class="dark:border-surface3 dark:bg-surface1 mb-6 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
					>
						<div class="flex items-center justify-between">
							<h3 class="text-base font-semibold">Filters</h3>
							<button
								onclick={() => {
									usernameFilter = '';
									emailFilter = '';
									projectFilter = '';
								}}
								class="button text-xs"
							>
								Clear all
							</button>
						</div>
						<div class="dark:border-surface3 grid grid-cols-1 gap-4 md:grid-cols-3">
							<div class="dark:border-surface3 flex flex-col gap-1">
								<label for="username-select" class="text-sm"> Username </label>
								<Select
									id="username-select"
									class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner dark:bg-black"
									options={usernameOptions}
									selected={usernameFilter}
									onSelect={handleUsernameSelect}
									position="top"
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label for="email-select" class="text-sm"> Email </label>
								<Select
									id="email-select"
									class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner dark:bg-black"
									options={emailOptions}
									selected={emailFilter}
									onSelect={handleEmailSelect}
									position="top"
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label for="project-select" class="text-sm"> Project Name </label>
								<Select
									id="project-select"
									class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner dark:bg-black"
									options={projectOptions}
									selected={projectFilter}
									onSelect={handleProjectSelect}
									position="top"
								/>
							</div>
						</div>
					</div>
				{/if}

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="size-8 animate-spin text-blue-600" />
					</div>
				{:else if filteredThreads.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="size-24 text-gray-200 dark:text-gray-700" />
						<h3 class="mt-4 text-lg font-semibold text-gray-400 dark:text-gray-600">
							{#if searchTerm}
								No threads found
							{:else}
								No threads available
							{/if}
						</h3>
						<p class="mt-2 text-sm text-gray-400 dark:text-gray-600">
							{#if searchTerm}
								Try adjusting your search terms.
							{:else}
								Threads will appear here once they are created.
							{/if}
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['name', 'userName', 'userEmail', 'projectName', 'created']}
						onSelectRow={handleViewThread}
						headers={[
							{
								title: 'Name',
								property: 'name'
							},
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
							},
							{
								title: 'Created',
								property: 'created'
							}
						]}
						headerClasses={[
							{
								property: 'name',
								class: 'w-4/12 min-w-sm'
							}
						]}
						sortable={['name', 'userName', 'userEmail', 'projectName', 'created']}
					>
						{#snippet actions(thread)}
							<button
								class="icon-button hover:text-blue-500"
								onclick={(e) => {
									e.stopPropagation();
									handleViewThread(thread);
								}}
								title="View Thread"
							>
								<Eye class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, thread)}
							{#if property === 'name'}
								<span>{formatThreadName(thread)}</span>
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
	<title>Obot | Admin - Chat Threads</title>
</svelte:head>

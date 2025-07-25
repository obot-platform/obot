<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { AdminService, type ProjectThread, type Project, type OrgUser } from '$lib/services';
	import { Search, Eye, LoaderCircle, RefreshCcw, MessageCircle, Filter, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';

	let threads = $state<ProjectThread[]>([]);
	let filteredThreads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	// Get unique options from thread data
	let usernameOptions = $derived.by(() => {
		const usernames = new Set<string>();
		threads.forEach((thread) => {
			const user = userMap.get(thread.userID || '');
			if (user?.displayName) {
				usernames.add(user.displayName);
			}
		});
		return Array.from(usernames).sort();
	});

	let emailOptions = $derived.by(() => {
		const emails = new Set<string>();
		threads.forEach((thread) => {
			const user = userMap.get(thread.userID || '');
			if (user?.email) {
				emails.add(user.email);
			}
		});
		return Array.from(emails).sort();
	});

	let projectOptions = $derived.by(() => {
		const projectNames = new Set<string>();
		threads.forEach((thread) => {
			const projectName = projectMap.get(thread.projectID || '') || thread.projectID;
			if (projectName) {
				projectNames.add(projectName);
			}
		});
		return Array.from(projectNames).sort();
	});
	let loading = $state(true);
	let searchTerm = $state('');
	let showFilters = $state(false);
	let usernameFilter = $state('');
	let emailFilter = $state('');
	let projectFilter = $state('');
	let showUsernameDropdown = $state(false);
	let showEmailDropdown = $state(false);
	let showProjectDropdown = $state(false);
	let sortField = $state<'created' | 'name' | 'userID' | 'projectID' | 'userName' | 'userEmail'>(
		'created'
	);
	let sortDirection = $state<'asc' | 'desc'>('desc');

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
					user?.email?.toLowerCase().includes(term)
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
		goto(`/v2/admin/chat-threads/${thread.id}`);
	}

	function formatThreadName(thread: ProjectThread) {
		return thread.name || 'Unnamed Thread';
	}

	function handleFilterFocus(filterType: 'username' | 'email' | 'project') {
		switch (filterType) {
			case 'username':
				showUsernameDropdown = true;
				showEmailDropdown = false;
				showProjectDropdown = false;
				break;
			case 'email':
				showUsernameDropdown = false;
				showEmailDropdown = true;
				showProjectDropdown = false;
				break;
			case 'project':
				showUsernameDropdown = false;
				showEmailDropdown = false;
				showProjectDropdown = true;
				break;
		}
	}

	function handleFilterBlur() {
		// Delay hiding dropdowns to allow for clicks
		setTimeout(() => {
			showUsernameDropdown = false;
			showEmailDropdown = false;
			showProjectDropdown = false;
		}, 150);
	}

	function selectFilterOption(value: string, filterType: 'username' | 'email' | 'project') {
		switch (filterType) {
			case 'username':
				usernameFilter = value;
				showUsernameDropdown = false;
				break;
			case 'email':
				emailFilter = value;
				showEmailDropdown = false;
				break;
			case 'project':
				projectFilter = value;
				showProjectDropdown = false;
				break;
		}
	}
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold">Chat Threads</h1>
				<div class="flex items-center gap-4">
					<div class="relative">
						<Search class="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-gray-400" />
						<input
							type="text"
							placeholder="Search threads..."
							bind:value={searchTerm}
							class="w-64 rounded-md border border-gray-300 bg-white px-10 py-2 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-800"
						/>
					</div>
					<button
						onclick={() => (showFilters = !showFilters)}
						class="flex items-center gap-2 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
					>
						<Filter class="size-4" />
						Filters
					</button>
					<button
						onclick={loadThreads}
						disabled={loading}
						class="button-primary flex gap-2 text-sm font-normal"
					>
						{#if loading}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							<RefreshCcw class="size-4" />
						{/if}
						Refresh
					</button>
				</div>
			</div>

			{#if showFilters}
				<div
					class="flex flex-col gap-4 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
				>
					<div class="flex items-center justify-between">
						<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300">Filters</h3>
						<button
							onclick={() => {
								usernameFilter = '';
								emailFilter = '';
								projectFilter = '';
							}}
							class="text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
						>
							Clear all
						</button>
					</div>
					<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
						<div class="flex flex-col gap-1">
							<label
								for="username-filter"
								class="text-xs font-medium text-gray-600 dark:text-gray-400"
							>
								Username
							</label>
							<div class="relative">
								<input
									id="username-filter"
									type="text"
									placeholder="Filter by username..."
									bind:value={usernameFilter}
									onfocus={() => handleFilterFocus('username')}
									onblur={handleFilterBlur}
									readonly
									class="w-full cursor-pointer rounded-md border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
								/>
								{#if usernameFilter}
									<button
										onclick={() => (usernameFilter = '')}
										class="absolute top-1/2 right-2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
										title="Clear username filter"
									>
										<X class="size-4" />
									</button>
								{/if}
								{#if showUsernameDropdown && usernameOptions.length > 0}
									<div
										class="absolute top-full right-0 left-0 z-10 mt-1 max-h-48 overflow-y-auto rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-600 dark:bg-gray-700"
									>
										{#each usernameOptions as option (option)}
											<button
												class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-600"
												onclick={() => selectFilterOption(option, 'username')}
											>
												{option}
											</button>
										{/each}
									</div>
								{/if}
							</div>
						</div>
						<div class="flex flex-col gap-1">
							<label
								for="email-filter"
								class="text-xs font-medium text-gray-600 dark:text-gray-400"
							>
								Email
							</label>
							<div class="relative">
								<input
									id="email-filter"
									type="text"
									placeholder="Filter by email..."
									bind:value={emailFilter}
									onfocus={() => handleFilterFocus('email')}
									onblur={handleFilterBlur}
									readonly
									class="w-full cursor-pointer rounded-md border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
								/>
								{#if emailFilter}
									<button
										onclick={() => (emailFilter = '')}
										class="absolute top-1/2 right-2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
										title="Clear email filter"
									>
										<X class="size-4" />
									</button>
								{/if}
								{#if showEmailDropdown && emailOptions.length > 0}
									<div
										class="absolute top-full right-0 left-0 z-10 mt-1 max-h-48 overflow-y-auto rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-600 dark:bg-gray-700"
									>
										{#each emailOptions as option (option)}
											<button
												class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-600"
												onclick={() => selectFilterOption(option, 'email')}
											>
												{option}
											</button>
										{/each}
									</div>
								{/if}
							</div>
						</div>
						<div class="flex flex-col gap-1">
							<label
								for="project-filter"
								class="text-xs font-medium text-gray-600 dark:text-gray-400"
							>
								Project Name
							</label>
							<div class="relative">
								<input
									id="project-filter"
									type="text"
									placeholder="Filter by project..."
									bind:value={projectFilter}
									onfocus={() => handleFilterFocus('project')}
									onblur={handleFilterBlur}
									readonly
									class="w-full cursor-pointer rounded-md border border-gray-300 bg-white px-3 py-2 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
								/>
								{#if projectFilter}
									<button
										onclick={() => (projectFilter = '')}
										class="absolute top-1/2 right-2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
										title="Clear project filter"
									>
										<X class="size-4" />
									</button>
								{/if}
								{#if showProjectDropdown && projectOptions.length > 0}
									<div
										class="absolute top-full right-0 left-0 z-10 mt-1 max-h-48 overflow-y-auto rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-600 dark:bg-gray-700"
									>
										{#each projectOptions as option (option)}
											<button
												class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-600"
												onclick={() => selectFilterOption(option, 'project')}
											>
												{option}
											</button>
										{/each}
									</div>
								{/if}
							</div>
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
					data={filteredThreads}
					fields={['name', 'userName', 'userEmail', 'projectID', 'created']}
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
							property: 'projectID'
						},
						{
							title: 'Created',
							property: 'created'
						}
					]}
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
							<span class="font-medium">{formatThreadName(thread)}</span>
						{:else if property === 'userName'}
							{@const user = userMap.get(thread.userID || '')}
							<span class="text-sm text-gray-600 dark:text-gray-400">
								{user?.displayName || '-'}
							</span>
						{:else if property === 'userEmail'}
							{@const user = userMap.get(thread.userID || '')}
							<span class="text-sm text-gray-600 dark:text-gray-400">
								{user?.email || '-'}
							</span>
						{:else if property === 'projectID'}
							<span class="text-sm text-gray-600 dark:text-gray-400">
								{thread.projectID ? projectMap.get(thread.projectID) || thread.projectID : '-'}
							</span>
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
</Layout>

<svelte:head>
	<title>Obot | Admin - Chat Threads</title>
</svelte:head>

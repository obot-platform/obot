# Code Style Guide

## Api/Data State (`SWR`)

Api State is managed 100% with `axios` and `SWR`.

When creating any api call, first create a route in the ApiRoutes object in `~/lib/routers/apiRoutes.ts`.

```ts
// ~/lib/routers/apiRoutes.ts

const ApiRoutes = {
    ...
    namespace: {
        route: (id, { queryParam }) => buildUrl(`/path/to/route/${id}`, { queryParam })
    }
}
```

### Queries (GET Requests)

Actual fetching logic is handled via Api Services in `~/lib/service/api`.

> Note: GET calls should always be coupled with a `key` and a `revalidate` method!

```ts
// ~/lib/services/api/namespaceService.ts

async function fetchData(id: string, queryParam: Nullish<string>) {
	const url = ApiRoutes.namespace.route(id, { queryParam }).url;

	return await request({ url, method: "GET" });
}
fetchData.key = (id: Nullish<string>, queryParam: Nullish<string>) => {
	if (!id) return null; // return null if no id is provided, this will prevent `SWR` from triggering the request

	// notice the above only checks for the existence of `id` and NOT `queryParam`.
	// this is because `queryParam` is not a required parameter for the `fetchData` function, but `id` is.

	return {
		url: ApiRoutes.namespace.route(id, { queryParam }).path, // always use the url path as a unique identifier (this also makes it easier to revalidate as needed

		// add all other dependencies to the key to ensure there are no cache collisions
		id,
		queryParam,
	};
};

// this revalidate method allows us to invalidate the cache for `fetchData` from anywhere in the application
fetchData.revalidate = createRevalidate(ApiRoutes.namespace.route);

export const ApiService = { fetchData };
```

We then use `useSWR` to cache the data and manage the api state.

```tsx
// ~/components/namespace/Component.tsx

const Component = ({ id, queryParam }) => {
	const { data, isLoading, error } = useSWR(
		ApiService.fetchData.key(id, queryParam),
		({ id, queryParam }) => ApiService.fetchData(id, queryParam) // id will always be defined here because if it's not, the key will return null and the request will not be triggered
	);

	return <div>...</div>;
};

const OtherRandomComponent = () => {
	return (
		<div>
			<Button
				// we can target a specific cache instance to revalidate
				onClick={() => ApiService.fetchData.revalidate(id, queryParam)}
				// or we can invalidate all instances for a given URL like so
				onClick={() => ApiService.fetchData.revalidate("(.*)", "(.*)")}
			>
				Revalidate
			</Button>
		</div>
	);
};
```

### Mutations (POST Requests)

Mutation methods do not require a `key` or `revalidate` method.

```ts
// ~/lib/services/api/namespaceService.ts

async function createData(data: CreateData) {
	const url = ApiRoutes.namespace.createData().url;

	return await request({ url, method: "POST", data });
}

export const ApiService = { createData };
```

When using them, it's usually best to wrap them in a `useAsync` hook to get access to various helpers.

```tsx
const MyComponent = () => {
	const { data, isLoading, error } = useAsync(ApiService.createData, {
		onSuccess: () => {
			// ...logic
		},
		onError: () => {
			// ...logic
		},
	});

	return <div>...</div>;
};
```

## Application State Management (`Zustand`)

> 90% of the time `SWR` and custom hooks are more than enough to handle all things state management. Zustand should ONLY be used for things that are inherently complex or need to be shared across the entire application.

Large state management libraries get way out of hand extremely easily. In order to mitigate this there are certain criteria that must be met to justify using a zustand store.

This criteria must always be met when using Zustand:

- (Always) Logic/state is **NOT** data/api related. We use `SWR` for all data/api related logic and using anything else will break the integrity of the api cache. Only Client-application logic should be handled with zustand.

At least 1 of the below criteria should be met:

- Logic/state is inherently complex and should be encapsulated
- Logic/state is inherently global
- Logic/state requires React render optimization

### Examples

#### Global State Management

When using zustand globally you can usually just create a store normally:

```ts
// ~/lib/store/global-store.ts

type GlobalStore = {...}

export const useGlobalStore = create<GlobalStore>()((set, get) => ({
    ...
}))
```

If this store relies on some sort of data state, you can tape them together like so

```tsx
// ~/lib/store/global-store.ts

type GlobalStore = {...}

const useGlobalStore = create<GlobalStore>()((set, get) => ({
    ...
}))

// ~/lib/components/...
const GlobalProvider = ({ children, ...props }: { children: React.ReactNode }) => {
    // ...SWR and data logic
    const { data } = useSWR(...)

    const { init } = useGlobalStore()

    // if we can find a way to produce the same effect here without duct taping data/application
    // stores together, I'm all for it.... I hate this
    useEffect(() => {
        init(data)
    }, [data, init])

    // notice no context is needed here because the store is already inherently global
    return children
}

// ~/lib/routes/root.tsx

const App = () => {
    return (
        ...providers
        <GlobalProvider>
            <Outlet />
        </GlobalProvider>
    )
}
```

#### Local State Management

When using zustand locally you will need to use the `createStore` function.

```tsx
// ~/lib/store/local-store.ts

type LocalStore = {...}

export const initLocalStore = (...params) => createStore<LocalStore>((set, get) => ({
    ...params,
    ...
}))

// ~/lib/hooks/namespace/use-init-local-store.ts
const useInitLocalStore = (...params) => {
    const [store] = useState(() => initLocalStore(...params))

    // any logic that needs to be done when the store is initialized


    return [
        useStore(store), // reactive store, can be used right away
        store // store instance, can be used to pass to context providers
    ]
}


// ~/lib/components/...

const StoreConsumer = ({ children, ...props }) => {
    const [store] = useInitLocalStore(...props)

    // store is avallable locally

    // you can also pass store properties to context providers if needed
}
```

When you need to pass the store to a context provider you can do so like so:

```tsx
// ~/lib/components/...

const StoreProvider = ({ children, ...props }) => {
	const [_, storeInstance] = useInitLocalStore(...props);

	return (
		<LocalStoreContext.Provider value={storeInstance}>
			{children}
		</LocalStoreContext.Provider>
	);
};

const useLocalStore = () => {
	const store = useContext(LocalStoreContext);

	if (!store) throw new Error("Store not found");

	return store;
};
```

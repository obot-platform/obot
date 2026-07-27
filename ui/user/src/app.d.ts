// See https://kit.svelte.dev/docs/types#app
// for information about these interfaces

declare module '*.svg?raw' {
	const content: string;
	export default content;
}

declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

// Widen synthesized `Pathname` / `PathnameWithSearchOrHash` so runtime-built root-relative URLs
// (e.g. from `URL`) can be passed to `resolve()` after `as \`/${string}\``.
declare module '$app/types' {
	export interface AppTypes {
		Pathname(): `/${string}`;
	}
}

export {};

import { forceError } from "~/lib/utils/forceError";

export function handleTry<TResponse>(fn: () => TResponse) {
	try {
		return [null, fn()] as const;
	} catch (e) {
		return [forceError(e), null] as const;
	}
}

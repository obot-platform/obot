// Hands a newly created API key's one-time secret from the create form to the key's detail page,
// which reveals it. The secret is kept only in memory (not in the URL or history state) and is
// cleared as soon as the detail page takes it.
let pending: { id: string; key: string } | undefined;

export const pendingApiKeyReveal = {
	set(id: string | number, key: string) {
		pending = { id: String(id), key };
	},
	take(id: string | number | undefined): string | undefined {
		if (!pending || id === undefined || pending.id !== String(id)) return undefined;
		const { key } = pending;
		pending = undefined;
		return key;
	}
};

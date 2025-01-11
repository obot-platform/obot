export const getAliasFrom = (text: Nullish<string>) => {
	if (!text) return "";

	return text.toLowerCase().replace(/[^a-z0-9-]+/g, "-");
};

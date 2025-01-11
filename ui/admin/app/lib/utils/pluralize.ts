export const pluralize = (
	count: number,
	singular: string,
	plural = singular + "s"
) => `${count === 1 ? singular : plural}`;

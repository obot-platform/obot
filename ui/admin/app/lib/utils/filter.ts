import { isAfter, isBefore, isSameDay } from "date-fns";

export const filterByCreatedRange = <T extends { created: string }>(
	items: T[],
	start: string,
	end?: string | null
) => {
	const startDate = new Date(start);
	const endDate = end ? new Date(end) : undefined;
	return items.filter((item) => {
		const createdDate = new Date(item.created);

		if (endDate) {
			const withinStart =
				isAfter(createdDate, startDate) || isSameDay(createdDate, startDate);
			const withinEnd =
				isBefore(createdDate, endDate) || isSameDay(createdDate, endDate);
			return withinStart && withinEnd;
		}
		return isSameDay(createdDate, startDate);
	});
};

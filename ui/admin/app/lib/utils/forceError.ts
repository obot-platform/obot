export const forceError = (
	error: unknown,
	fallbackMessage = "Something went wrong"
) => {
	if (error instanceof Error) return error;

	if (typeof error === "string") return new Error(error);

	return new Error(fallbackMessage, { cause: error });
};

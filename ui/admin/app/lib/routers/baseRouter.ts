export const BaseUrl = (route: string = "/") => {
	if (typeof window === "undefined") return "";

	return window.location.origin + "/admin" + route;
};
export const DomainUrl = (route: string = "/") => {
	if (typeof window === "undefined") return "";

	return window.location.origin + route;
};

export const ApiUrl = () => {
	if (import.meta.env.VITE_API_URL) return import.meta.env.VITE_API_URL;

	if (typeof window === "undefined") return "";

	return window.location.origin + "/api";
};

export const ConsumptionUrl = (route: string) => {
	return window.location.protocol + "//" + window.location.host + route;
};

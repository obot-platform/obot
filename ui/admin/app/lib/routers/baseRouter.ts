export const BaseUrl = () => {
    if (typeof window === "undefined") return "";

    return window.location.origin + "/admin";
};
export const DomainUrl = () => {
    if (typeof window === "undefined") return "";

    return window.location.origin;
};

export const ApiUrl = () => {
    if (import.meta.env.VITE_API_URL) return import.meta.env.VITE_API_URL;

    if (typeof window === "undefined") return "";

    return window.location.origin + "/api";
};

export const ConsumptionUrl = (route: string) => {
    return window.location.protocol + "//" + window.location.host + route;
};

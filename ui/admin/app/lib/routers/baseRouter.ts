export const BaseUrl = () => {
    if (typeof window === "undefined") return "";

    return window.location.origin + "/admin";
};
export const DomainUrl = () => {
    if (typeof window === "undefined") return "";

    return window.location.origin;
};

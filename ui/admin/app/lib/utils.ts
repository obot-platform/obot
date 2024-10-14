import { type ClassValue, clsx } from "clsx";
import queryString from "query-string";
import { twMerge } from "tailwind-merge";
import { ZodSchema } from "zod";

export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs));
}

export const noop = () => null;

export const truncate = (str: string, maxLength: number) => {
    if (str.length <= maxLength) return str;
    return str.slice(0, maxLength) + "...";
};

/**
 * Parses the query params from the url
 * @returns a Zod safeParse result
 */
export const parseQueryParams = <T extends object>(
    url: string,
    schema: ZodSchema<T>
) => {
    // use safeParse to allow the caller to handle the error
    return schema.safeParse(queryString.parseUrl(url).query);
};

const pluralize = (count: number, singular: string, plural: string) =>
    `${count === 1 ? singular : plural}`;

export const timeSince = (date: Date) => {
    const seconds = Math.floor((new Date().getTime() - date.getTime()) / 1000);

    let interval = seconds / 31536000;

    if (interval > 1) {
        return (
            Math.floor(interval) + " " + pluralize(interval, "year", "years")
        );
    }
    interval = seconds / 2592000;
    if (interval > 1) {
        return (
            Math.floor(interval) + " " + pluralize(interval, "month", "months")
        );
    }
    interval = seconds / 86400;
    if (interval > 1) {
        return Math.floor(interval) + " " + pluralize(interval, "day", "days");
    }
    interval = seconds / 3600;
    if (interval > 1) {
        return (
            Math.floor(interval) + " " + pluralize(interval, "hour", "hours")
        );
    }
    interval = seconds / 60;
    if (interval > 1) {
        return (
            Math.floor(interval) +
            " " +
            pluralize(interval, "minute", "minutes")
        );
    }
    return Math.floor(seconds) + " " + pluralize(seconds, "second", "seconds");
};

export const getErrorMessage = (error: unknown) => {
    if (!error) return;

    if (error instanceof Error) return error.message;
    if (typeof error === "string") return error;
    return "Something went wrong";
};

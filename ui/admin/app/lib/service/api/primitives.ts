// TODO: Add default configurations with auth tokens, etc. When ready
import axios from "axios";

export const ResponseHeaders = {
    RunId: "x-otto-run-id",
    ThreadId: "x-otto-thread-id",
} as const;

const internalFetch = axios.request;

export const request: typeof internalFetch = (config) => {
    return internalFetch({
        adapter: "fetch",
        ...config,
    });
};

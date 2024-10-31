import { AxiosError } from "axios";
import { toast } from "sonner";

const toastError = (error: unknown) => {
    if (error instanceof AxiosError) {
        if (typeof error.response?.data === "string")
            toast.error(error.response?.data);
    }

    if (error instanceof Error) {
        toast.error(error.message);
    }

    return "An unknown error occurred";
};

export const ErrorService = { toastError };

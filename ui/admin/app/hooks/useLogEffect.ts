import { useEffect } from "react";

export function useLogEffect(...deps: unknown[]) {
    useEffect(() => {
        console.log(...deps);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);
}

declare global {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    type Todo = any;

    type Nullish<T> = T | null | undefined;
}

export {};

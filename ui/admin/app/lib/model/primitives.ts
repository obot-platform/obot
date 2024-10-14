export type EntityMeta = {
    created: string;
    deleted?: string; // date
    id: string;
    links: Record<string, string>;
};

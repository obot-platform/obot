export type EntityMeta<
    TMetadata extends Record<string, string> = Record<string, string>,
> = {
    created: string;
    deleted?: string; // date
    id: string;
    links: Record<string, string>;
    metadata?: TMetadata;
    type?: string;
};

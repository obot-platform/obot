export type Metadata = Record<string, string>;
export type MetaLinks = Record<string, string>;

export type EntityMeta<
    TMetadata extends Metadata = Metadata,
    TLinks extends MetaLinks = MetaLinks,
> = {
    created: string;
    deleted?: string; // date
    id: string;
    links?: TLinks;
    metadata?: TMetadata;
    type?: string;
};

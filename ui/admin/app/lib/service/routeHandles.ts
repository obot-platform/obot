type BreadcrumbItem = {
    content?: React.ReactNode;
    href?: string;
};

type BreadcrumbProps = {
    pathname: string;
    search: string;
};

export type RouteHandle = {
    breadcrumb?: (props: BreadcrumbProps) => BreadcrumbItem[];
};

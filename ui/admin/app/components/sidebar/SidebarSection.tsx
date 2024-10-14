import { Link } from "react-router-dom";

import { Button } from "~/components/ui/button";

type SidebarSectionProps = {
    title: string;
    linkTo: string;
    children?: React.ReactNode;
    icon?: React.ReactNode;
};

export function SidebarSection({
    title,
    linkTo,
    children,
    icon,
}: SidebarSectionProps) {
    return (
        <div>
            <Button
                asChild
                variant="ghost"
                className="h-full w-full rounded-none"
            >
                <Link to={linkTo}>
                    <p className="flex text-lg w-full gap-2 py-2 items-center">
                        {icon}
                        <span>{title}</span>
                    </p>
                </Link>
            </Button>
            <div className="px-3">{children}</div>
        </div>
    );
}

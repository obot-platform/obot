import { CircleAlertIcon } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert";

export function WarningAlert({
    title,
    description,
}: {
    title: string;
    description: React.ReactNode;
}) {
    return (
        <Alert variant="default">
            <CircleAlertIcon className="w-4 h-4 !text-warning" />
            <AlertTitle>{title}</AlertTitle>
            <AlertDescription>{description}</AlertDescription>
        </Alert>
    );
}

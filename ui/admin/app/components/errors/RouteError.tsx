import { ErrorResponse } from "@remix-run/react";

import { OttoLogo } from "~/components/branding/OttoLogo";
import { Button } from "~/components/ui/button";
import {
    Card,
    CardContent,
    CardDescription,
    CardFooter,
    CardHeader,
    CardTitle,
} from "~/components/ui/card";

export function RouteError({ error }: { error: ErrorResponse }) {
    return (
        <div className="flex min-h-screen w-full items-center justify-center p-4">
            <Card className="w-96">
                <CardHeader className="mx-4">
                    <OttoLogo />
                </CardHeader>
                <CardContent className="space-y-2 text-center border-b mb-4">
                    <CardTitle>Oops! {error.status}</CardTitle>
                    <CardDescription>{error.statusText}</CardDescription>
                    <p className="text-sm text-muted-foreground">
                        {error.data}
                    </p>
                </CardContent>
                <CardFooter>
                    <Button
                        className="w-full"
                        variant="secondary"
                        onClick={() => window.location.reload()}
                    >
                        Try Again
                    </Button>
                </CardFooter>
            </Card>
        </div>
    );
}

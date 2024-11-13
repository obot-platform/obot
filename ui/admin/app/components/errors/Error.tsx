import { Link } from "@remix-run/react";
import { ArrowLeft, HomeIcon } from "lucide-react";

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

export function Error({ error }: { error: Error }) {
    return (
        <div className="flex min-h-screen w-full items-center justify-center p-4">
            <Card className="w-96">
                <CardHeader className="mx-4">
                    <OttoLogo />
                </CardHeader>
                <CardContent className="space-y-2 text-center border-b mb-4">
                    <CardTitle>Oops! An error occurred</CardTitle>
                    <CardDescription>{error.message}</CardDescription>
                    <p className="text-sm text-muted-foreground">
                        Please try again later or contact support if the problem
                        persists.
                    </p>
                </CardContent>
                <CardFooter className="flex gap-4">
                    <Button className="w-full" variant="secondary" asChild>
                        <Link to="/">
                            <HomeIcon className="mr-2" /> Go home
                        </Link>
                    </Button>
                    <Button
                        className="w-full"
                        variant="secondary"
                        onClick={() => {
                            window.location.reload();
                        }}
                    >
                        <ArrowLeft className="mr-2" /> Go back
                    </Button>
                </CardFooter>
            </Card>
        </div>
    );
}

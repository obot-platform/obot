import { ArrowLeft, HomeIcon, RefreshCw } from "lucide-react";
import { useNavigate } from "react-router";

import { ObotLogo } from "~/components/branding/ObotLogo";
import { Button } from "~/components/ui/button";
import {
    Card,
    CardContent,
    CardDescription,
    CardFooter,
    CardHeader,
    CardTitle,
} from "~/components/ui/card";
import { Link } from "~/components/ui/link";

export function Error({ error }: { error: Error }) {
    const navigate = useNavigate();

    return (
        <div className="flex min-h-screen w-full items-center justify-center p-4">
            <Card className="w-96">
                <CardHeader className="mx-4">
                    <ObotLogo />
                </CardHeader>
                <CardContent className="space-y-2 text-center border-b mb-4">
                    <CardTitle>Oops! An error occurred</CardTitle>
                    <CardDescription>{error.message}</CardDescription>
                    <p className="text-sm text-muted-foreground">
                        Please try again later or contact support if the problem
                        persists.
                    </p>
                </CardContent>

                <CardFooter className="flex flex-col gap-4">
                    <Button
                        className="w-full"
                        onClick={() => navigate(0)}
                        startContent={<RefreshCw />}
                    >
                        Try again
                    </Button>

                    <div className="flex items-center gap-4 w-full">
                        <Link
                            as="button"
                            className="w-full"
                            variant="secondary"
                            to="/"
                        >
                            <HomeIcon /> Go home
                        </Link>

                        <Button
                            className="w-full"
                            variant="secondary"
                            onClick={() => navigate(-1)}
                            startContent={<ArrowLeft />}
                        >
                            Go back
                        </Button>
                    </div>
                </CardFooter>
            </Card>
        </div>
    );
}

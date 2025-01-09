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
				<CardContent className="mb-4 space-y-2 border-b text-center">
					<CardTitle>Oops! An error occurred</CardTitle>
					<CardDescription>{error.message}</CardDescription>
					<p className="text-sm text-muted-foreground">
						Please try again later or contact support if the problem persists.
					</p>
				</CardContent>

				<CardFooter className="flex flex-col gap-4">
					<Button
						className="w-full"
						onClick={() => navigate(0)}
						startContent={<RefreshCw />}
					>
						Try Again
					</Button>

					<div className="flex w-full items-center gap-4">
						<Link as="button" className="w-full" variant="secondary" to="/">
							<HomeIcon /> Go Home
						</Link>

						<Button
							className="w-full"
							variant="secondary"
							onClick={() => navigate(-1)}
							startContent={<ArrowLeft />}
						>
							Go Back
						</Button>
					</div>
				</CardFooter>
			</Card>
		</div>
	);
}

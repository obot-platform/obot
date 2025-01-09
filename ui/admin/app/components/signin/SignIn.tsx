import { FaGoogle } from "react-icons/fa";

import { cn } from "~/lib/utils";

import { ObotLogo } from "~/components/branding/ObotLogo";
import { Button } from "~/components/ui/button";
import {
	Card,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "~/components/ui/card";

interface SignInProps {
	className?: string;
}

export function SignIn({ className }: SignInProps) {
	return (
		<div className="flex min-h-screen w-full items-center justify-center p-4">
			<Card className={cn("flex w-96 flex-col justify-between", className)}>
				<CardHeader>
					<CardTitle className="flex items-center justify-center">
						<ObotLogo />
					</CardTitle>
					<CardDescription className="mx-auto w-3/4 pt-4 text-center">
						Please sign in using the button below.
					</CardDescription>
				</CardHeader>
				<CardFooter className="border-t pt-4">
					<Button
						variant="secondary"
						className="w-full"
						onClick={() => {
							window.location.href = "/oauth2/start?rd=/admin/";
						}}
					>
						<FaGoogle className="mr-2" />
						Sign In with Google
					</Button>
				</CardFooter>
			</Card>
		</div>
	);
}

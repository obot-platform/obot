import { ObotLogo } from "~/components/branding/ObotLogo";
import { Button } from "~/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardFooter,
	CardHeader,
} from "~/components/ui/card";

export function Unauthorized() {
	return (
		<div className="flex min-h-screen w-full items-center justify-center p-4">
			<Card className="w-96">
				<CardHeader className="mx-4">
					<ObotLogo />
				</CardHeader>
				<CardContent className="mb-4 space-y-2 border-b text-center">
					<CardDescription className="text-center">
						You are not authorized to access this page. Please sign in with an
						authorized account or contact your administrator.
					</CardDescription>
				</CardContent>
				<CardFooter>
					<Button
						className="w-full"
						variant="secondary"
						onClick={() => {
							window.location.href = "/oauth2/sign_out?rd=/admin/";
						}}
					>
						Sign Out
					</Button>
				</CardFooter>
			</Card>
		</div>
	);
}

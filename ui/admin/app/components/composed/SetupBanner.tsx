import { Link, useLocation } from "react-router";
import { $path } from "safe-routes";

import { assetUrl, cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { useAuthProviders } from "~/hooks/auth-providers/useAuthProviders";
import { useModelProviders } from "~/hooks/model-providers/useModelProviders";

export function SetupBanner() {
	const { configured: modelProviderConfigured } = useModelProviders();
	const { configured: authProviderConfigured } = useAuthProviders();
	const location = useLocation();

	const steps = [
		{
			step: "Configure Model Provider",
			configured: modelProviderConfigured,
			path: $path("/model-providers"),
			description: "To use Agents and Workflows, configure a Model Provider",
			label: "Model Provider",
		},
		{
			step: "Configure Auth Provider",
			configured: authProviderConfigured,
			path: $path("/auth-providers"),
			description: "To support multiple users, configure an Auth Provider.",
			label: "Auth Provider",
		},
	].filter((step) => !step.configured);

	const isOnSetupPage = steps.some((step) =>
		location.pathname.includes(step.path)
	);

	if (!steps.length || isOnSetupPage) return null;

	return (
		<div className="w-full">
			<div className="mx-8 mt-4 flex justify-center overflow-hidden rounded-xl bg-secondary py-4">
				<div className="relative flex min-h-36 w-[calc(100%-4rem)] max-w-screen-md flex-row items-center justify-between gap-4 rounded-sm">
					<div className="absolute opacity-5 md:left-[-3.0rem] md:top-[-1.75rem] md:opacity-45">
						<img
							alt="Obot Alert"
							className="md:h-[17.5rem] md:w-[17.5rem]"
							src={assetUrl("logo/obot-icon-surprised-yellow.svg")}
						/>
					</div>

					<div className="relative z-10 flex flex-col gap-1 md:ml-64">
						<h3 className="mb-0.5">
							Wait! You&apos;ve still got some setup to do!
						</h3>

						<ul>
							{steps.map((step) => (
								<li key={step.step}>
									<p className="mb-2 text-sm font-light">
										<b className="font-semibold">{step.label}: </b>
										{step.description}
									</p>
								</li>
							))}
						</ul>

						<div className="flex flex-row flex-wrap gap-2">
							{steps.map((step) => (
								<Button
									className={cn("mt-0 w-fit px-10", {
										"flex-1": steps.length > 1,
									})}
									variant="warning"
									key={step.step}
								>
									<Link to={step.path}>{step.step}</Link>
								</Button>
							))}
						</div>
					</div>
				</div>
			</div>
		</div>
	);
}

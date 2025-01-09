import { Link, useLocation } from "react-router";
import { $path } from "safe-routes";

import { assetUrl } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { useModelProviders } from "~/hooks/model-providers/useModelProviders";

export function FirstModelProviderBanner() {
	const { configured: modelProviderConfigured } = useModelProviders();
	const location = useLocation();
	const isModelsProviderPage = location.pathname.includes(
		$path("/model-providers")
	);

	return isModelsProviderPage || modelProviderConfigured ? null : (
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
							Wait! You need to set up a Model Provider!
						</h3>
						<p className="mb-2 text-sm font-light">
							You&apos;re almost there! To start creating or using Obot&apos;s
							features, you&apos;ll need access to an LLM (Large Language Model){" "}
							<b>Model Provider</b>. Luckily, we support a variety of providers
							to help get you started.
						</p>
						<Button className="mt-0 w-fit px-10" variant="warning">
							<Link to={$path("/model-providers")}>Get Started</Link>
						</Button>
					</div>
				</div>
			</div>
		</div>
	);
}

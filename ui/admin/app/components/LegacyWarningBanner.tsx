import { AlertTriangle } from "lucide-react";

export function LegacyWarningBanner() {
	return (
		<div className="border-b border-yellow-200 bg-yellow-50 px-4 py-3">
			<div className="flex items-center justify-center">
				<AlertTriangle className="mr-2 h-5 w-5 text-yellow-600" />
				<span className="text-sm text-yellow-800">
					<strong>Warning:</strong> This is Obot legacy admin view and it has
					been deprecated. Use{" "}
					<a
						href="/admin"
						className="font-medium text-yellow-900 underline hover:text-yellow-700"
					>
						/admin
					</a>{" "}
					instead.
				</span>
			</div>
		</div>
	);
}

import { useState } from "react";

import { Model } from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { Switch } from "~/components/ui/switch";

export function UpdateModelActive({
	model,
	onChange,
}: {
	model: Model;
	onChange?: (active: boolean) => void;
}) {
	const [active, setActive] = useState(model.active);
	const handleModelStatusChange = (checked: boolean) => {
		ModelApiService.updateModel(model.id, {
			...model,
			active: checked,
		});
		setActive(checked);
		onChange?.(checked);
	};

	return <Switch checked={active} onCheckedChange={handleModelStatusChange} />;
}

import { useEffect, useState } from "react";
import { useStore } from "zustand";

import { createMessageStore } from "~/lib/store/chat/message-store";

type Config = {
	init?: boolean;
};

export const useInitMessageStore = (
	threadId: Nullish<string>,
	config?: Config
) => {
	const { init: _init = true } = config ?? {};
	const [storeObj] = useState(() => createMessageStore());
	const store = useStore(storeObj);

	const { init, reset } = store;
	useEffect(() => {
		if (!threadId || !_init) return;

		init(threadId);

		return () => reset();
	}, [init, reset, threadId, _init]);

	return store;
};

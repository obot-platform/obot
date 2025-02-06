import { SearchIcon } from "lucide-react";
import { useState } from "react";

import { Input } from "~/components/ui/input";
import { useDebounce } from "~/hooks/useDebounce";

export function SearchInput({
	onChange,
	placeholder = "Search...",
}: {
	onChange: (value: string) => void;
	placeholder?: string;
}) {
	const [searchQuery, setSearchQuery] = useState("");
	const debounceOnChange = useDebounce(onChange, 300);
	return (
		<div className="relative">
			<SearchIcon className="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 transform text-gray-400" />
			<Input
				type="text"
				placeholder={placeholder}
				value={searchQuery}
				onChange={(e) => {
					setSearchQuery(e.target.value);
					debounceOnChange(e.target.value);
				}}
				className="w-64 pl-10"
			/>
		</div>
	);
}

import { DndContext, DragEndEvent, UniqueIdentifier } from "@dnd-kit/core";
import * as Primitive from "@dnd-kit/sortable";
import { CSS, Transform } from "@dnd-kit/utilities";
import { GripVerticalIcon } from "lucide-react";

import { cn } from "~/lib/utils";

import { AnimatePresence } from "~/components/ui/animate";

const SortableContext = Primitive.SortableContext;

type SortableProps = {
	children: React.ReactNode;
	id: UniqueIdentifier;
	className?: string;
	isHandle?: boolean;
};

function Sortable({ children, id, className, isHandle = true }: SortableProps) {
	const { attributes, listeners, setNodeRef, transform, transition, active } =
		Primitive.useSortable({ id });

	const style = {
		transform: CSS.Transform.toString({
			...transform,
			scaleX: 1,
			scaleY: 1,
		} as Transform),
		transition,
	};

	const isDragging = active?.id === id;

	const handleProps = isHandle ? { ...attributes, ...listeners } : {};

	return (
		<div
			ref={setNodeRef}
			{...handleProps}
			style={{
				...style,
				zIndex: isDragging ? 50 : undefined,
			}}
			className={cn(className)}
		>
			{children}
		</div>
	);
}

type SortableHandleProps = {
	children?: React.ReactNode;
	id: string;
	className?: string;
};

function SortableHandle({ children, id, className }: SortableHandleProps) {
	const { attributes, listeners } = Primitive.useSortable({
		id,
		resizeObserverConfig: { disabled: true },
	});

	const handleProps = { ...attributes, ...listeners };

	return children ? (
		<div {...handleProps} className={className}>
			{children}
		</div>
	) : (
		<GripVerticalIcon
			{...handleProps}
			className={cn("cursor-grab active:cursor-grabbing", className)}
		/>
	);
}

const arrayMove = Primitive.arrayMove;

type SortableListProps<T> = {
	items: T[];
	renderItem: (item: T, index: number) => React.ReactNode;
	getKey: (item: T) => string;
	onChange: (newItems: T[]) => void;
	isHandle?: boolean;
};

function SortableList<T>({
	items,
	renderItem,
	getKey,
	onChange,
	isHandle,
}: SortableListProps<T>) {
	const handleDragEnd = (event: DragEndEvent) => {
		const { active, over } = event;

		if (!active || !over) return;

		const activeIndex = items.findIndex((item) => getKey(item) === active.id);
		const overIndex = items.findIndex((item) => getKey(item) === over?.id);

		const newItems = arrayMove(items, activeIndex, overIndex);

		onChange(newItems);
	};

	return (
		<DndContext onDragEnd={handleDragEnd}>
			<SortableContext items={items.map((item) => getKey(item))}>
				<AnimatePresence>
					{items.map((item, index) => (
						<Sortable key={getKey(item)} id={getKey(item)} isHandle={isHandle}>
							{renderItem(item, index)}
						</Sortable>
					))}
				</AnimatePresence>
			</SortableContext>
		</DndContext>
	);
}

export { Sortable, SortableContext, SortableHandle, arrayMove, SortableList };

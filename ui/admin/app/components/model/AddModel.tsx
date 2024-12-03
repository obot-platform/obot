import { PlusIcon } from "lucide-react";
import { useState } from "react";

import { ModelForm } from "~/components/model/ModelForm";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";

export function AddModel() {
    const [open, setOpen] = useState(false);

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                <Button variant="outline" startContent={<PlusIcon />}>
                    Add Model
                </Button>
            </DialogTrigger>

            <DialogContent>
                <DialogTitle>Create Model</DialogTitle>

                <DialogDescription hidden>Create Model</DialogDescription>

                <ModelForm onSubmit={() => setOpen(false)} />
            </DialogContent>
        </Dialog>
    );
}

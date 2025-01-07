import {
    CreateEmailReceiver,
    EmailReceiver,
    UpdateEmailReceiver,
} from "~/lib/model/email-receivers";
import { EntityList } from "~/lib/model/primitives";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getEmailReceivers() {
    const { data } = await request<EntityList<EmailReceiver>>({
        url: ApiRoutes.emailReceivers.getEmailReceivers().url,
    });

    return data.items ?? [];
}
getEmailReceivers.key = () => ({
    url: ApiRoutes.emailReceivers.getEmailReceivers().url,
});

async function getEmailReceiverById(id: string) {
    const { data } = await request<EmailReceiver>({
        url: ApiRoutes.emailReceivers.getEmailReceiverById(id).url,
    });

    return data;
}
getEmailReceiverById.key = (id: Nullish<string>) => {
    if (!id) return null;

    return {
        url: ApiRoutes.emailReceivers.getEmailReceiverById(id).url,
        emailReceiverId: id,
    };
};

async function createEmailReceiver(emailReceiver: CreateEmailReceiver) {
    const { data } = await request<EmailReceiver>({
        url: ApiRoutes.emailReceivers.createEmailReceiver().url,
        method: "POST",
        data: emailReceiver,
    });

    return data;
}

async function updateEmailReceiver(
    id: string,
    emailReceiver: UpdateEmailReceiver
) {
    const { data } = await request<EmailReceiver>({
        url: ApiRoutes.emailReceivers.updateEmailReceiver(id).url,
        method: "PUT",
        data: emailReceiver,
    });

    return data;
}

async function deleteEmailReceiver(id: string) {
    await request({
        url: ApiRoutes.emailReceivers.deleteEmailReceiver(id).url,
        method: "DELETE",
    });
}

export const EmailReceiverApiService = {
    getEmailReceivers,
    getEmailReceiverById,
    createEmailReceiver,
    updateEmailReceiver,
    deleteEmailReceiver,
};

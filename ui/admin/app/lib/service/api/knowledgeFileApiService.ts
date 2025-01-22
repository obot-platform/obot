import { KnowledgeFile, KnowledgeFileNamespace } from "~/lib/model/knowledge";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getKnowledgeFiles(
	namespace: KnowledgeFileNamespace,
	entityId: string
) {
	const res = await request<{ items: KnowledgeFile[] }>({
		url: ApiRoutes.knowledgeFiles.getKnowledgeFiles(namespace, entityId).url,
		errorMessage: "Failed to fetch knowledge for agent",
	});

	return res.data.items;
}
getKnowledgeFiles.key = (
	namespace?: Nullish<KnowledgeFileNamespace>,
	entityId?: Nullish<string>
) => {
	if (!namespace || !entityId) return null;

	return {
		url: ApiRoutes.knowledgeFiles.getKnowledgeFiles(namespace, entityId).path,
		entityId,
		namespace,
	};
};

async function addKnowledgeFiles(
	namespace: KnowledgeFileNamespace,
	agentId: string,
	file: File
) {
	const res = await request<KnowledgeFile>({
		url: ApiRoutes.knowledgeFiles.addKnowledgeFile(
			namespace,
			agentId,
			file.name
		).url,
		method: "POST",
		data: await file.arrayBuffer(),
		headers: { "Content-Type": "application/x-www-form-urlencoded" },
		errorMessage: "Failed to add knowledge to agent",
	});
	return res.data;
}

async function deleteKnowledgeFile(
	namespace: KnowledgeFileNamespace,
	agentId: string,
	fileName: string
) {
	await request({
		url: ApiRoutes.knowledgeFiles.deleteKnowledgeFile(
			namespace,
			agentId,
			fileName
		).url,
		method: "DELETE",
		errorMessage: "Failed to delete knowledge from agent",
	});
}

async function reingestFile(
	namespace: KnowledgeFileNamespace,
	agentId: string,
	fileID: string
) {
	const { url } = ApiRoutes.knowledgeFiles.reingestKnowledgeFile(
		namespace,
		agentId,
		fileID
	);

	const res = await request<KnowledgeFile>({
		url,
		method: "POST",
		errorMessage: "Failed to reingest knowledge file",
	});

	return res.data;
}

export const KnowledgeFileService = {
	getKnowledgeFiles,
	addKnowledgeFiles,
	deleteKnowledgeFile,
	reingestFile,
};

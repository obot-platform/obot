import type { ImagePullSecret, ImagePullSecretType } from '$lib/services';

export type ImagePullSecretFormState = {
	type: ImagePullSecretType;
	enabled: boolean;
	displayName: string;
	server: string;
	username: string;
	password: string;
	roleARN: string;
	region: string;
	issuerURL: string;
	audience: string;
	refreshSchedule: string;
};

export const defaultECRAudience = 'sts.amazonaws.com';

const awsAccountIDPlaceholder = '<aws-account-id>';
const issuerPlaceholder = '<issuer-without-https>';
const defaultAWSPartition = 'aws';

export function defaultForm(type: ImagePullSecretType): ImagePullSecretFormState {
	return {
		type,
		enabled: true,
		displayName: '',
		server: '',
		username: '',
		password: '',
		roleARN: '',
		region: '',
		issuerURL: '',
		audience: '',
		refreshSchedule: ''
	};
}

export function formFromSecret(secret: ImagePullSecret): ImagePullSecretFormState {
	return {
		type: secret.manifest.type ?? 'basic',
		enabled: secret.manifest.enabled,
		displayName: secret.manifest.displayName ?? '',
		server: secret.manifest.basic?.server ?? '',
		username: secret.manifest.basic?.username ?? '',
		password: '',
		roleARN: secret.manifest.ecr?.roleARN ?? '',
		region: secret.manifest.ecr?.region ?? '',
		issuerURL: secret.manifest.ecr?.issuerURL ?? '',
		audience: secret.manifest.ecr?.audience ?? '',
		refreshSchedule: secret.manifest.ecr?.refreshSchedule ?? ''
	};
}

export function displayName(secret: ImagePullSecret) {
	return secret.manifest.displayName || secret.id;
}

export function statusLabel(secret: ImagePullSecret) {
	if (!secret.manifest.enabled) return 'Disabled';
	if (secret.status?.lastError) return 'Error';
	if (secret.status?.lastSuccessTime) return 'Ready';
	return 'Pending';
}

export function statusMessage(secret?: ImagePullSecret) {
	return secret?.status?.lastError ?? '';
}

export function statusClass(secret: ImagePullSecret) {
	switch (statusLabel(secret)) {
		case 'Ready':
			return 'bg-green-500/10 text-green-700 dark:text-green-300';
		case 'Error':
			return 'bg-red-500/10 text-red-700 dark:text-red-300';
		case 'Disabled':
			return 'bg-gray-500/10 text-gray-600 dark:text-gray-300';
		default:
			return 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-300';
	}
}

export function issuerHostPath(issuerURL: string) {
	return (
		issuerURL
			.trim()
			.replace(/^https:\/\//, '')
			.replace(/\/+$/, '') || issuerPlaceholder
	);
}

export function ecrTrustPolicyJSON(
	roleARN: string,
	issuerURL: string,
	subject: string,
	audience: string
) {
	const roleMatch = roleARN.trim().match(/^arn:([^:]+):iam::([0-9]{12}):role\/.+$/);
	const partition = roleMatch?.[1] || defaultAWSPartition;
	const accountID = roleMatch?.[2] || awsAccountIDPlaceholder;
	const issuer = issuerHostPath(issuerURL);
	const tokenAudience = audience.trim() || defaultECRAudience;
	const tokenSubject = subject.trim() || '<service-account-subject>';

	return JSON.stringify(
		{
			Version: '2012-10-17',
			Statement: [
				{
					Effect: 'Allow',
					Principal: {
						Federated: `arn:${partition}:iam::${accountID}:oidc-provider/${issuer}`
					},
					Action: 'sts:AssumeRoleWithWebIdentity',
					Condition: {
						StringEquals: {
							[`${issuer}:sub`]: tokenSubject,
							[`${issuer}:aud`]: tokenAudience
						}
					}
				}
			]
		},
		null,
		2
	);
}

export function ecrPolicyJSON() {
	return JSON.stringify(
		{
			Version: '2012-10-17',
			Statement: [
				{
					Effect: 'Allow',
					Action: ['ecr:GetAuthorizationToken'],
					Resource: '*'
				},
				{
					Effect: 'Allow',
					Action: [
						'ecr:BatchCheckLayerAvailability',
						'ecr:BatchGetImage',
						'ecr:GetDownloadUrlForLayer'
					],
					Resource: '*'
				}
			]
		},
		null,
		2
	);
}

// Localized text for the guardrail types the server registers. The server describes each type,
// its settings and its test findings in English (GET /llm-gateway/guardrails/providers and the
// test endpoint). The UI translates them by stable keys: the type ID and the field name. A string
// is translated only while the server's English still matches the English in the catalog, so a
// type the UI doesn't know, or text the server has since changed, shows the server's English
// rather than a stale translation.
import { m, type Locale } from '$lib/i18n';
import type { GuardrailField, GuardrailProvider } from './guardrails';

type Text = (options?: { locale?: Locale }) => string;

interface TypeText {
	// Integrations are named after their product, so most have no label of their own.
	label?: Text;
	description: Text;
	fields: Record<string, { label: Text; description?: Text }>;
}

const TYPE_TEXT: Record<string, TypeText> = {
	regex: {
		label: (o) => m.guardrails_type_regex_label({}, o),
		description: (o) => m.guardrails_type_regex_description({}, o),
		fields: {
			piiTemplate: {
				label: (o) => m.guardrails_type_regex_field_pii_template_label({}, o),
				description: (o) => m.guardrails_type_regex_field_pii_template_description({}, o)
			},
			patterns: {
				label: (o) => m.guardrails_type_regex_field_patterns_label({}, o),
				description: (o) =>
					m.guardrails_type_regex_field_patterns_description({ example: 'EMPLOYEE_ID=E\\d{6}' }, o)
			},
			caseInsensitive: { label: (o) => m.guardrails_type_regex_field_case_insensitive_label({}, o) }
		}
	},
	'llm-judge': {
		label: (o) => m.guardrails_type_llm_judge_label({}, o),
		description: (o) => m.guardrails_type_llm_judge_description({}, o),
		fields: {
			definition: {
				label: (o) => m.guardrails_type_llm_judge_field_definition_label({}, o),
				description: (o) => m.guardrails_type_llm_judge_field_definition_description({}, o)
			}
		}
	},
	secrets: {
		label: (o) => m.guardrails_type_secrets_label({}, o),
		description: (o) => m.guardrails_type_secrets_description({}, o),
		fields: {
			generic: {
				label: (o) => m.guardrails_type_secrets_field_generic_label({}, o),
				description: (o) => m.guardrails_type_secrets_field_generic_description({}, o)
			},
			minEntropy: {
				label: (o) => m.guardrails_type_secrets_field_min_entropy_label({}, o),
				description: (o) => m.guardrails_type_secrets_field_min_entropy_description({}, o)
			}
		}
	},
	'aws-bedrock': {
		description: (o) => m.guardrails_type_aws_bedrock_description({}, o),
		fields: {
			region: {
				label: (o) => m.guardrails_type_aws_bedrock_field_region_label({}, o),
				description: (o) => m.guardrails_type_aws_bedrock_field_region_description({}, o)
			},
			guardrailId: {
				label: (o) => m.guardrails_type_aws_bedrock_field_guardrail_id_label({}, o),
				description: (o) => m.guardrails_type_aws_bedrock_field_guardrail_id_description({}, o)
			},
			guardrailVersion: {
				label: (o) => m.guardrails_type_aws_bedrock_field_guardrail_version_label({}, o),
				description: (o) => m.guardrails_type_aws_bedrock_field_guardrail_version_description({}, o)
			},
			accessKeyId: {
				label: (o) => m.guardrails_type_aws_bedrock_field_access_key_id_label({}, o),
				description: (o) => m.guardrails_type_aws_bedrock_field_access_key_id_description({}, o)
			},
			secretAccessKey: {
				label: (o) => m.guardrails_type_aws_bedrock_field_secret_access_key_label({}, o)
			},
			sessionToken: {
				label: (o) => m.guardrails_type_aws_bedrock_field_session_token_label({}, o),
				description: (o) => m.guardrails_type_aws_bedrock_field_session_token_description({}, o)
			},
			endpoint: {
				label: (o) => m.guardrails_type_aws_bedrock_field_endpoint_label({}, o),
				description: (o) => m.guardrails_type_aws_bedrock_field_endpoint_description({}, o)
			}
		}
	},
	'azure-content-safety': {
		description: (o) => m.guardrails_type_azure_content_safety_description({}, o),
		fields: {
			endpoint: {
				label: (o) => m.guardrails_type_azure_content_safety_field_endpoint_label({}, o),
				description: (o) => m.guardrails_type_azure_content_safety_field_endpoint_description({}, o)
			},
			apiKey: { label: (o) => m.guardrails_type_azure_content_safety_field_api_key_label({}, o) },
			categories: {
				label: (o) => m.guardrails_type_azure_content_safety_field_categories_label({}, o),
				description: (o) =>
					m.guardrails_type_azure_content_safety_field_categories_description({}, o)
			},
			severityThreshold: {
				label: (o) => m.guardrails_type_azure_content_safety_field_severity_threshold_label({}, o),
				description: (o) =>
					m.guardrails_type_azure_content_safety_field_severity_threshold_description({}, o)
			},
			promptShields: {
				label: (o) => m.guardrails_type_azure_content_safety_field_prompt_shields_label({}, o),
				description: (o) =>
					m.guardrails_type_azure_content_safety_field_prompt_shields_description({}, o)
			}
		}
	},
	'azure-language-pii': {
		description: (o) => m.guardrails_type_azure_language_pii_description({}, o),
		fields: {
			endpoint: {
				label: (o) => m.guardrails_type_azure_language_pii_field_endpoint_label({}, o),
				description: (o) => m.guardrails_type_azure_language_pii_field_endpoint_description({}, o)
			},
			apiKey: { label: (o) => m.guardrails_type_azure_language_pii_field_api_key_label({}, o) },
			language: {
				label: (o) => m.guardrails_type_azure_language_pii_field_language_label({}, o),
				description: (o) => m.guardrails_type_azure_language_pii_field_language_description({}, o)
			},
			domain: {
				label: (o) => m.guardrails_type_azure_language_pii_field_domain_label({}, o),
				description: (o) => m.guardrails_type_azure_language_pii_field_domain_description({}, o)
			},
			categories: {
				label: (o) => m.guardrails_type_azure_language_pii_field_categories_label({}, o),
				description: (o) => m.guardrails_type_azure_language_pii_field_categories_description({}, o)
			},
			minConfidence: {
				label: (o) => m.guardrails_type_azure_language_pii_field_min_confidence_label({}, o),
				description: (o) =>
					m.guardrails_type_azure_language_pii_field_min_confidence_description({}, o)
			}
		}
	},
	'crowdstrike-aidr': {
		description: (o) => m.guardrails_type_crowdstrike_aidr_description({}, o),
		fields: {
			apiToken: {
				label: (o) => m.guardrails_type_crowdstrike_aidr_field_api_token_label({}, o),
				description: (o) => m.guardrails_type_crowdstrike_aidr_field_api_token_description({}, o)
			},
			baseURL: {
				label: (o) => m.guardrails_type_crowdstrike_aidr_field_base_url_label({}, o),
				description: (o) => m.guardrails_type_crowdstrike_aidr_field_base_url_description({}, o)
			},
			appID: {
				label: (o) => m.guardrails_type_crowdstrike_aidr_field_app_id_label({}, o),
				description: (o) => m.guardrails_type_crowdstrike_aidr_field_app_id_description({}, o)
			},
			collectorInstanceID: {
				label: (o) => m.guardrails_type_crowdstrike_aidr_field_collector_instance_id_label({}, o),
				description: (o) =>
					m.guardrails_type_crowdstrike_aidr_field_collector_instance_id_description({}, o)
			}
		}
	},
	'google-model-armor': {
		description: (o) => m.guardrails_type_google_model_armor_description({}, o),
		fields: {
			project: {
				label: (o) => m.guardrails_type_google_model_armor_field_project_label({}, o),
				description: (o) => m.guardrails_type_google_model_armor_field_project_description({}, o)
			},
			location: {
				label: (o) => m.guardrails_type_google_model_armor_field_location_label({}, o),
				description: (o) => m.guardrails_type_google_model_armor_field_location_description({}, o)
			},
			template: { label: (o) => m.guardrails_type_google_model_armor_field_template_label({}, o) },
			serviceAccountJSON: {
				label: (o) => m.guardrails_type_google_model_armor_field_service_account_json_label({}, o),
				description: (o) =>
					m.guardrails_type_google_model_armor_field_service_account_json_description({}, o)
			},
			endpoint: {
				label: (o) => m.guardrails_type_google_model_armor_field_endpoint_label({}, o),
				description: (o) => m.guardrails_type_google_model_armor_field_endpoint_description({}, o)
			}
		}
	},
	lakera: {
		description: (o) => m.guardrails_type_lakera_description({}, o),
		fields: {
			apiKey: { label: (o) => m.guardrails_type_lakera_field_api_key_label({}, o) },
			projectID: {
				label: (o) => m.guardrails_type_lakera_field_project_id_label({}, o),
				description: (o) => m.guardrails_type_lakera_field_project_id_description({}, o)
			},
			minConfidence: {
				label: (o) => m.guardrails_type_lakera_field_min_confidence_label({}, o),
				description: (o) => m.guardrails_type_lakera_field_min_confidence_description({}, o)
			},
			baseURL: {
				label: (o) => m.guardrails_type_lakera_field_base_url_label({}, o),
				description: (o) => m.guardrails_type_lakera_field_base_url_description({}, o)
			}
		}
	},
	presidio: {
		description: (o) => m.guardrails_type_presidio_description({}, o),
		fields: {
			analyzerURL: {
				label: (o) => m.guardrails_type_presidio_field_analyzer_url_label({}, o),
				description: (o) => m.guardrails_type_presidio_field_analyzer_url_description({}, o)
			},
			language: {
				label: (o) => m.guardrails_type_presidio_field_language_label({}, o),
				description: (o) => m.guardrails_type_presidio_field_language_description({}, o)
			},
			entities: {
				label: (o) => m.guardrails_type_presidio_field_entities_label({}, o),
				description: (o) => m.guardrails_type_presidio_field_entities_description({}, o)
			},
			scoreThreshold: {
				label: (o) => m.guardrails_type_presidio_field_score_threshold_label({}, o),
				description: (o) => m.guardrails_type_presidio_field_score_threshold_description({}, o)
			},
			apiKey: {
				label: (o) => m.guardrails_type_presidio_field_api_key_label({}, o),
				description: (o) => m.guardrails_type_presidio_field_api_key_description({}, o)
			}
		}
	},
	patronus: {
		description: (o) => m.guardrails_type_patronus_description({}, o),
		fields: {
			apiKey: { label: (o) => m.guardrails_type_patronus_field_api_key_label({}, o) },
			evaluators: {
				label: (o) => m.guardrails_type_patronus_field_evaluators_label({}, o),
				description: (o) => m.guardrails_type_patronus_field_evaluators_description({}, o)
			},
			baseURL: { label: (o) => m.guardrails_type_patronus_field_base_url_label({}, o) }
		}
	}
};

function localize(server: string, text: Text | undefined): string {
	if (!text || text({ locale: 'en' }) !== server) return server;
	return text();
}

/** A guardrail type's name, as shown to the user. */
export function guardrailTypeLabel(provider: Pick<GuardrailProvider, 'type' | 'label'>): string {
	return localize(provider.label, TYPE_TEXT[provider.type]?.label);
}

/** A guardrail type's description, as shown to the user. */
export function guardrailTypeDescription(
	provider: Pick<GuardrailProvider, 'type' | 'description'>
): string {
	return localize(provider.description, TYPE_TEXT[provider.type]?.description);
}

/** The label of a guardrail type's setting, as shown to the user. */
export function guardrailFieldLabel(type: string, field: Pick<GuardrailField, 'name' | 'label'>) {
	return localize(field.label, TYPE_TEXT[type]?.fields[field.name]?.label);
}

/** The help text of a guardrail type's setting, as shown to the user. */
export function guardrailFieldDescription(
	type: string,
	field: Pick<GuardrailField, 'name' | 'description'>
): string {
	return localize(field.description ?? '', TYPE_TEXT[type]?.fields[field.name]?.description);
}

// Confidence levels as the server writes them in findings (Model Armor and Bedrock).
const CONFIDENCE: Record<string, () => string> = {
	none: m.guardrails_finding_confidence_none,
	low: m.guardrails_finding_confidence_low,
	medium: m.guardrails_finding_confidence_medium,
	high: m.guardrails_finding_confidence_high,
	'low and above': m.guardrails_finding_confidence_low_and_above,
	'medium and above': m.guardrails_finding_confidence_medium_and_above
};

const confidence = (level: string) => CONFIDENCE[level]?.() ?? level;

const FINDING_TEXT: Record<string, () => string> = {
	'Violates the policy': m.guardrails_finding_violates_policy,
	'Prompt attack detected in the user prompt': m.guardrails_finding_prompt_attack,
	'Indirect prompt attack detected in a tool result': m.guardrails_finding_indirect_prompt_attack,
	'Prompt injection or jailbreak attempt': m.guardrails_finding_prompt_injection,
	'Responsible AI filter matched': m.guardrails_finding_responsible_ai_matched,
	'Malicious URL detected': m.guardrails_finding_malicious_url,
	'Child sexual abuse material detected': m.guardrails_finding_csam,
	'Malware detected': m.guardrails_finding_malware,
	'Sensitive data detected': m.guardrails_finding_sensitive_data,
	'Model Armor filter matched': m.guardrails_finding_model_armor_matched,
	'Blocked by CrowdStrike AIDR policy': m.guardrails_finding_crowdstrike_blocked,
	'Redacted by CrowdStrike AIDR policy': m.guardrails_finding_crowdstrike_redacted,
	'Blocked word': m.guardrails_finding_blocked_word,
	'Automated reasoning check failed': m.guardrails_finding_automated_reasoning_failed,
	'AWS Bedrock guardrail intervened': m.guardrails_finding_bedrock_intervened,
	'Flagged by Lakera Guard': m.guardrails_finding_lakera_flagged
};

// Findings that name what was found, most specific first. What they name (an entity type,
// detector or pattern label) comes from the guardrail and stays as it is.
const FINDING_PATTERNS: [RegExp, (match: string[]) => string][] = [
	[
		/^(.+) content detected \(severity (\d+)\)$/,
		([, category, severity]) => m.guardrails_finding_content_severity({ category, severity })
	],
	[
		/^Prompt injection or jailbreak attempt \((.+) confidence\)$/,
		([, level]) =>
			m.guardrails_finding_prompt_injection_confidence({ confidence: confidence(level) })
	],
	[
		/^(.+) content \((.+) confidence\)$/,
		([, label, level]) =>
			m.guardrails_finding_content_confidence({ label, confidence: confidence(level) })
	],
	[/^(.+) content$/, ([, label]) => m.guardrails_finding_content({ label })],
	[/^Denied topic: (.+)$/, ([, name]) => m.guardrails_finding_denied_topic({ name })],
	[/^(.+) word list match$/, ([, type]) => m.guardrails_finding_word_list_match({ type })],
	[/^(.+) pattern matched$/, ([, name]) => m.guardrails_finding_pattern_matched({ name })],
	[
		/^(.+) score (\S+) below threshold (\S+)$/,
		([, type, score, threshold]) =>
			m.guardrails_finding_score_below_threshold({ type, score, threshold })
	],
	[/^(.+) check failed$/, ([, type]) => m.guardrails_finding_check_failed({ type })],
	[
		/^Failed Patronus evaluator (.+)$/,
		([, name]) => m.guardrails_finding_patronus_failed({ name })
	],
	[/^(.+) detected$/, ([, label]) => m.guardrails_finding_detected({ label })]
];

/** A guardrail finding's message, as shown to the user. Messages the UI doesn't know pass through. */
export function guardrailFindingMessage(message: string): string {
	const exact = FINDING_TEXT[message];
	if (exact) return exact();
	for (const [pattern, format] of FINDING_PATTERNS) {
		const match = message.match(pattern);
		if (match) return format(match);
	}
	return message;
}

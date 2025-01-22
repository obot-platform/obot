{{/*
Return the chart name and version.
*/}}
{{- define "obot.chart" -}}
{{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}

{{/*
Expand the name of the chart.
*/}}
{{- define "obot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a fullname using the release name and the chart name.
*/}}
{{- define "obot.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "obot.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Create labels for the resources.
*/}}
{{- define "obot.labels" -}}
helm.sh/chart: {{ include "obot.chart" . }}
{{ include "obot.selectorLabels" . }}
{{- with .Chart.AppVersion }}
app.kubernetes.io/version: {{ . | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Create selector labels for the resources.
*/}}
{{- define "obot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "obot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "obot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "obot.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Set name of secret to use for credentials
*/}}
{{- define "obot.config.secretName" -}}
{{- if .Values.config.existingSecret -}}
{{- .Values.config.existingSecret -}}
{{- else -}}
{{ .Release.Name }}-config
{{- end -}}
{{- end -}}
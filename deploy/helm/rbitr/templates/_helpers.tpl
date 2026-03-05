{{/*
Expand the name of the chart.
*/}}
{{- define "rbitr.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "rbitr.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "rbitr.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "rbitr.labels" -}}
helm.sh/chart: {{ include "rbitr.chart" . }}
{{ include "rbitr.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "rbitr.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rbitr.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Gateway labels.
*/}}
{{- define "rbitr.gateway.labels" -}}
{{ include "rbitr.labels" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{/*
Gateway selector labels.
*/}}
{{- define "rbitr.gateway.selectorLabels" -}}
{{ include "rbitr.selectorLabels" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{/*
UI labels.
*/}}
{{- define "rbitr.ui.labels" -}}
{{ include "rbitr.labels" . }}
app.kubernetes.io/component: ui
{{- end }}

{{/*
UI selector labels.
*/}}
{{- define "rbitr.ui.selectorLabels" -}}
{{ include "rbitr.selectorLabels" . }}
app.kubernetes.io/component: ui
{{- end }}

{{/*
Service account name.
*/}}
{{- define "rbitr.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "rbitr.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
DATABASE_URL construction.
When externalDatabase.existingSecret is set, the job/deployment mounts it directly.
Otherwise, build the URL from discrete values.
*/}}
{{- define "rbitr.databaseURL" -}}
{{- if .Values.postgresql.enabled -}}
postgres://{{ .Values.postgresql.auth.username }}:{{ .Values.postgresql.auth.password }}@{{ include "rbitr.fullname" . }}-postgresql:5432/{{ .Values.postgresql.auth.database }}?sslmode=disable
{{- else -}}
postgres://{{ .Values.externalDatabase.user }}:{{ .Values.externalDatabase.password }}@{{ .Values.externalDatabase.host }}:{{ .Values.externalDatabase.port }}/{{ .Values.externalDatabase.database }}?sslmode={{ .Values.externalDatabase.sslmode }}
{{- end -}}
{{- end }}

{{/*
Name of the secret holding DATABASE_URL and other credentials.
*/}}
{{- define "rbitr.secretName" -}}
{{- if .Values.externalDatabase.existingSecret -}}
{{- .Values.externalDatabase.existingSecret }}
{{- else -}}
{{- include "rbitr.fullname" . }}
{{- end -}}
{{- end }}

{{/*
Key inside the secret for DATABASE_URL.
*/}}
{{- define "rbitr.secretDatabaseKey" -}}
{{- if .Values.externalDatabase.existingSecret -}}
{{- .Values.externalDatabase.existingSecretKey }}
{{- else -}}
DATABASE_URL
{{- end -}}
{{- end }}

{{/*
Gateway image reference.
*/}}
{{- define "rbitr.gateway.image" -}}
{{- $tag := .Values.gateway.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.gateway.image.repository $tag }}
{{- end }}

{{/*
UI image reference.
*/}}
{{- define "rbitr.ui.image" -}}
{{- $tag := .Values.ui.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.ui.image.repository $tag }}
{{- end }}

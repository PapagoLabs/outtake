{{- define "outtake.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "outtake.fullname" -}}
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

{{- define "outtake.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "outtake.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "outtake.selectorLabels" -}}
app.kubernetes.io/name: {{ include "outtake.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "outtake.httpPort" -}}
{{- $addr := .Values.outtake.listenAddr | toString -}}
{{- if contains ":" $addr -}}
{{- $parts := splitList ":" $addr -}}
{{- index $parts (sub (len $parts) 1) -}}
{{- else -}}
{{- .Values.service.port | toString -}}
{{- end -}}
{{- end }}

{{- define "outtake.storageBackend" -}}
{{- if or .Values.backends.seaweedfs.enabled .Values.backends.rustfs.enabled -}}
s3
{{- else -}}
{{- .Values.outtake.storageBackend -}}
{{- end -}}
{{- end }}

{{- define "outtake.databaseBackend" -}}
{{- if or .Values.backends.cockroach.enabled .Values.backends.cnpg.enabled -}}
postgres
{{- else -}}
{{- .Values.outtake.databaseBackend -}}
{{- end -}}
{{- end }}

{{- define "outtake.s3Endpoint" -}}
{{- if .Values.outtake.s3.endpoint -}}
{{- .Values.outtake.s3.endpoint -}}
{{- else if .Values.backends.seaweedfs.enabled -}}
http://{{ .Release.Name }}-seaweedfs-s3:8333
{{- else if .Values.backends.rustfs.enabled -}}
http://{{ .Release.Name }}-rustfs:9000
{{- else -}}
{{- .Values.outtake.s3.endpoint -}}
{{- end -}}
{{- end }}

{{- define "outtake.databaseUrl" -}}
{{- if .Values.outtake.databaseUrl -}}
{{- .Values.outtake.databaseUrl -}}
{{- else if .Values.backends.cockroach.enabled -}}
postgres://root@{{ .Release.Name }}-cockroachdb-public:26257/defaultdb?sslmode=disable
{{- else -}}
{{- .Values.outtake.databaseUrl -}}
{{- end -}}
{{- end }}

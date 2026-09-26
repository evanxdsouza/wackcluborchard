{{- define "orchard.labels" -}}
app.kubernetes.io/name: orchard
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "orchard.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end -}}

{{- define "orchard.registryHost" -}}
{{- if .Values.registry.host -}}
{{ .Values.registry.host }}
{{- else -}}
registry.{{ .Release.Namespace }}.svc.cluster.local:5000
{{- end -}}
{{- end -}}

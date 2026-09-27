{{- define "wackcluborchard.labels" -}}
app.kubernetes.io/name: wackcluborchard
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "wackcluborchard.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end -}}

{{- define "wackcluborchard.registryHost" -}}
{{- if .Values.registry.host -}}
{{ .Values.registry.host }}
{{- else -}}
registry.{{ .Release.Namespace }}.svc.cluster.local:5000
{{- end -}}
{{- end -}}

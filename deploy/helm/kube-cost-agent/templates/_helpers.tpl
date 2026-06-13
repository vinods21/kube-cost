{{- define "kube-cost-agent.fullname" -}}
{{- printf "%s-agent" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

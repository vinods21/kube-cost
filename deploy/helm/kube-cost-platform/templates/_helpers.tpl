{{- define "kube-cost-platform.name" -}}
{{- printf "%s-platform" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kube-cost-platform.labels" -}}
app.kubernetes.io/part-of: kube-cost
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end }}

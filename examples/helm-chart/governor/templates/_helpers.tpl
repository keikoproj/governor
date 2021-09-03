{{- define "image.version" }}
  {{- if .Values.reaper.imageVersion }}
    {{ .Values.reaper.imageVersion }}
  {{- else if (eq .Chart.AppVersion "0.3.0-aplha") }}
    0.3.0
  {{- else }}
    latest
  {{- end }}
{{- end }}

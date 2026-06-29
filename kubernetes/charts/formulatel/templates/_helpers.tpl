{{/*
Create the name of the application.
*/}}
{{- define "formulatel.fullname" -}}
{{- default .Release.Name ._selector -}}-formulatel
{{- end -}}

{{/*
Create the name of the current component.
*/}}
{{- define "formulatel.name" -}}
{{- default .Chart.Name ._selector -}}
{{- end -}}

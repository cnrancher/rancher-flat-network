{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}

{{- define "system_default_registry" -}}
{{- if .Values.global.cattle.systemDefaultRegistry -}}
{{- printf "%s/" .Values.global.cattle.systemDefaultRegistry -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "multus_cniconf_kubeconfig" -}}
{{- if eq .Values.clusterType "K3s" -}}
/var/lib/rancher/k3s/agent/etc/cni/net.d/multus.d/multus.kubeconfig
{{- else -}}
/etc/cni/net.d/multus.d/multus.kubeconfig
{{- end -}}
{{- end -}}

{{- define "multus_cniconf_host_path" -}}
{{- if eq .Values.clusterType "K3s" -}}
/var/lib/rancher/k3s/agent/etc/cni/net.d
{{- else -}}
/etc/cni/net.d
{{- end -}}
{{- end -}}


{{- define "multus_cnibin_host_path" -}}
{{- if eq .Values.clusterType "K3s" -}}
/var/lib/rancher/k3s/data/cni
{{- else -}}
/opt/cni/bin
{{- end -}}
{{- end -}}

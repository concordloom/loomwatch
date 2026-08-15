{{/*
Helpers for the loomwatch chart.

These reimplement the small subset of the Bitnami `common` library chart that
this chart actually uses. The chart deliberately carries no library
dependency: a monitoring component should be installable without dragging in
another chart's release cadence, and a self-contained chart is one file to read
rather than two repositories to correlate.
*/}}

{{/* Chart name, overridable. */}}
{{- define "loomwatch.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name.
Truncated at 63 characters because some Kubernetes name fields are limited by
the DNS label spec.
*/}}
{{- define "loomwatch.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "loomwatch.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride -}}
{{- end -}}

{{/* Render a value that may itself contain template directives. */}}
{{- define "loomwatch.tplvalues.render" -}}
{{- $value := typeIs "string" .value | ternary .value (.value | toYaml) }}
{{- if contains "{{" (toString $value) }}
{{- tpl $value .context }}
{{- else }}
{{- $value }}
{{- end }}
{{- end -}}

{{- define "loomwatch.labels" -}}
helm.sh/chart: {{ include "loomwatch.chart" . }}
{{ include "loomwatch.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: loomwatch
{{- if .Values.commonLabels }}
{{ include "loomwatch.tplvalues.render" (dict "value" .Values.commonLabels "context" $) }}
{{- end }}
{{- end -}}

{{- define "loomwatch.selectorLabels" -}}
app.kubernetes.io/name: {{ include "loomwatch.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "loomwatch.annotations" -}}
{{- if .Values.commonAnnotations }}
{{ include "loomwatch.tplvalues.render" (dict "value" .Values.commonAnnotations "context" $) }}
{{- end }}
{{- end -}}

{{/*
Image reference.
A digest, when given, replaces the tag entirely: the two together would let the
tag drift while the digest silently decided what actually runs.
*/}}
{{- define "loomwatch.image" -}}
{{- $registry := default .Values.image.registry .Values.global.imageRegistry -}}
{{- $tag := .Values.image.tag | toString -}}
{{- if .Values.image.digest -}}
{{- printf "%s/%s@%s" $registry .Values.image.repository (.Values.image.digest | toString) -}}
{{- else -}}
{{- printf "%s/%s:%s" $registry .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.imagePullSecrets" -}}
{{- $secrets := concat (.Values.global.imagePullSecrets | default list) (.Values.image.pullSecrets | default list) | uniq -}}
{{- if $secrets }}
imagePullSecrets:
{{- range $secrets }}
  {{- if kindIs "string" . }}
  - name: {{ . }}
  {{- else }}
  - {{ toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end -}}
{{- end -}}

{{- define "loomwatch.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "loomwatch.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.storageClass" -}}
{{- $sc := .Values.persistence.storageClass | default .Values.global.storageClass | default .Values.global.defaultStorageClass -}}
{{- if $sc -}}
{{- if (eq "-" $sc) -}}
storageClassName: ""
{{- else -}}
storageClassName: {{ $sc | quote }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.pvcName" -}}
{{- default (printf "%s-data" (include "loomwatch.fullname" .)) .Values.persistence.existingClaim -}}
{{- end -}}

{{/* Credentials secret: either the user's own, or one this chart manages. */}}
{{- define "loomwatch.secretName" -}}
{{- if .Values.auth.existingSecret -}}
{{- tpl .Values.auth.existingSecret . -}}
{{- else -}}
{{- printf "%s-credentials" (include "loomwatch.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.createSecret" -}}
{{- if not .Values.auth.existingSecret -}}true{{- end -}}
{{- end -}}

{{/*
Generated credentials must survive `helm upgrade`.
Without the lookup, every upgrade would mint a new password and token: the
panel would start rejecting the operator's saved password, and every Prometheus
scrape would begin returning 401 - a monitoring outage caused by the act of
upgrading the monitor.
*/}}
{{- define "loomwatch.lookupSecretValue" -}}
{{- $existing := (lookup "v1" "Secret" (include "loomwatch.namespace" .context) (include "loomwatch.secretName" .context)) -}}
{{- if and $existing $existing.data (index $existing.data .key) -}}
{{- index $existing.data .key | b64dec -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.adminPassword" -}}
{{- if .Values.auth.adminPassword -}}
{{- .Values.auth.adminPassword -}}
{{- else -}}
{{- $found := include "loomwatch.lookupSecretValue" (dict "context" . "key" .Values.auth.secretKeys.adminPasswordKey) -}}
{{- $found | default (randAlphaNum 28) -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.metricsToken" -}}
{{- if .Values.auth.metricsToken -}}
{{- .Values.auth.metricsToken -}}
{{- else -}}
{{- $found := include "loomwatch.lookupSecretValue" (dict "context" . "key" .Values.auth.secretKeys.metricsTokenKey) -}}
{{- $found | default (randAlphaNum 32) -}}
{{- end -}}
{{- end -}}

{{/* Affinity presets, equivalent to the ones common provides. */}}
{{- define "loomwatch.affinities.nodes" -}}
{{- $type := .type -}}
{{- if eq $type "soft" }}
preferredDuringSchedulingIgnoredDuringExecution:
  - preference:
      matchExpressions:
        - key: {{ .key }}
          operator: In
          values:
            {{- range .values }}
            - {{ . | quote }}
            {{- end }}
    weight: 1
{{- else if eq $type "hard" }}
requiredDuringSchedulingIgnoredDuringExecution:
  nodeSelectorTerms:
    - matchExpressions:
        - key: {{ .key }}
          operator: In
          values:
            {{- range .values }}
            - {{ . | quote }}
            {{- end }}
{{- end -}}
{{- end -}}

{{- define "loomwatch.affinities.pods" -}}
{{- $type := .type -}}
{{- $context := .context -}}
{{- if eq $type "soft" }}
preferredDuringSchedulingIgnoredDuringExecution:
  - podAffinityTerm:
      labelSelector:
        matchLabels: {{- (include "loomwatch.selectorLabels" $context) | nindent 10 }}
      topologyKey: kubernetes.io/hostname
    weight: 1
{{- else if eq $type "hard" }}
requiredDuringSchedulingIgnoredDuringExecution:
  - labelSelector:
      matchLabels: {{- (include "loomwatch.selectorLabels" $context) | nindent 8 }}
    topologyKey: kubernetes.io/hostname
{{- end -}}
{{- end -}}

{{/*
Resource presets.
Only the low end is defined. The collector polls a handful of HTTP endpoints on
a timer and writes a small SQLite file; anything above `small` would reserve
capacity that cannot be used.
*/}}
{{- define "loomwatch.resources.preset" -}}
{{- $presets := dict
  "none" dict
  "nano" (dict "requests" (dict "cpu" "20m" "memory" "64Mi" "ephemeral-storage" "50Mi") "limits" (dict "memory" "256Mi" "ephemeral-storage" "1Gi"))
  "micro" (dict "requests" (dict "cpu" "50m" "memory" "128Mi" "ephemeral-storage" "50Mi") "limits" (dict "memory" "384Mi" "ephemeral-storage" "1Gi"))
  "small" (dict "requests" (dict "cpu" "100m" "memory" "256Mi" "ephemeral-storage" "50Mi") "limits" (dict "memory" "512Mi" "ephemeral-storage" "2Gi"))
-}}
{{- if hasKey $presets .type -}}
{{- index $presets .type | toYaml -}}
{{- else -}}
{{- fail (printf "resourcesPreset %q is not known. Known presets: %s" .type (join ", " (keys $presets | sortAlpha))) -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.ingress.apiVersion" -}}
{{- if .Values.ingress.apiVersion -}}
{{- .Values.ingress.apiVersion -}}
{{- else if .Capabilities.APIVersions.Has "networking.k8s.io/v1" -}}
networking.k8s.io/v1
{{- else -}}
networking.k8s.io/v1beta1
{{- end -}}
{{- end -}}

{{- define "loomwatch.pdb.apiVersion" -}}
{{- if .Capabilities.APIVersions.Has "policy/v1/PodDisruptionBudget" -}}
policy/v1
{{- else -}}
policy/v1beta1
{{- end -}}
{{- end -}}

{{/*
Validation.
Every check below corresponds to a way the chart can be installed such that it
comes up and then quietly fails to do its job, which is worse than not starting.
*/}}
{{- define "loomwatch.validateValues" -}}
{{- $messages := list -}}
{{- $messages = append $messages (include "loomwatch.validateValues.replicaCount" .) -}}
{{- $messages = append $messages (include "loomwatch.validateValues.databasePath" .) -}}
{{- $messages = append $messages (include "loomwatch.validateValues.updateStrategy" .) -}}
{{- $messages = without $messages "" -}}
{{- $message := join "\n" $messages -}}
{{- if $message -}}
{{- printf "\nVALUES VALIDATION:\n%s" $message | fail -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.validateValues.replicaCount" -}}
{{- if gt (int .Values.replicaCount) 1 -}}
loomwatch: replicaCount
    loomwatch is a single writer against a SQLite file on a ReadWriteOnce
    volume. A second replica cannot attach the same volume, and if persistence
    is disabled it would instead double every provider poll against quotas that
    are themselves rate limited.
{{- end -}}
{{- end -}}

{{- define "loomwatch.validateValues.databasePath" -}}
{{- if .Values.persistence.enabled -}}
{{- if not (hasPrefix (printf "%s/" .Values.persistence.mountPath) .Values.databasePath) -}}
loomwatch: databasePath
    databasePath ({{ .Values.databasePath }}) is outside the mounted volume
    ({{ .Values.persistence.mountPath }}), so the database would be written to
    the container filesystem and lost on every restart - taking provider
    accounts and the panel password with it.
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "loomwatch.validateValues.updateStrategy" -}}
{{- if and .Values.persistence.enabled (not .Values.persistence.existingClaim) (eq .Values.updateStrategy.type "RollingUpdate") (has "ReadWriteOnce" .Values.persistence.accessModes) -}}
loomwatch: updateStrategy.type
    RollingUpdate with a ReadWriteOnce volume schedules the new Pod before the
    old one releases the volume. If it lands on another node the rollout
    deadlocks on a Multi-Attach error and never completes. Use Recreate, or an
    access mode that permits multi-node attachment.
{{- end -}}
{{- end -}}

{{/*
Missing provider credentials are deliberately NOT a validation failure, only a
warning in NOTES.txt. Accounts can also be added through the dashboard, and for
some providers that is the only way to add a second one - so an install with no
credentials in values is a legitimate starting point, not a mistake.
*/}}

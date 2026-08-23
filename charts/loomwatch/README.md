<!--
  The parameter tables below are generated from the @param annotations in
  values.yaml. Do not edit them by hand - run:

      python3 charts/loomwatch/hack/gen-params.py

  Pass --check to verify they are current without rewriting.
-->

# loomwatch

LLM subscription quota tracking with a Prometheus-first interface. Exports how
much of each provider plan is left, per account and per quota window, so you
find out before a workload stops rather than after.

This is a fork of [onWatch](https://github.com/onllm-dev/onWatch) aimed at
running as a service rather than on a laptop. See
[why the fork exists](#differences-from-upstream).

## TL;DR

```console
helm install loomwatch oci://ghcr.io/concordloom/charts/loomwatch \
  --set auth.providers.MINIMAX_API_KEY=... \
  --set auth.providers.ZAI_API_KEY=... \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.prometheusRule.enabled=true
```

## Introduction

Consumption is easy to measure and rarely what you need. What matters is the
*remainder*: only the provider's own quota API knows the limit of a plan, and it
is expressed as a share of that limit. That makes the thresholds in this chart
natural rather than guessed — 100 is not "a lot of tokens", it is the quota.

The chart deploys the collector, wires it to Prometheus and, optionally, ships
alerting rules and a dashboard.

## Prerequisites

- Kubernetes 1.23+
- Helm 3.8+ (OCI support)
- A PersistentVolume provisioner, unless `persistence.enabled=false`
- Prometheus Operator, for `metrics.serviceMonitor` and `metrics.prometheusRule`

## Installing the Chart

```console
helm install my-release oci://ghcr.io/concordloom/charts/loomwatch
```

## Uninstalling the Chart

```console
helm uninstall my-release
```

The PersistentVolumeClaim is not removed by `helm uninstall`. Provider accounts
added through the dashboard live there, so deleting it is a separate, deliberate
act:

```console
kubectl delete pvc my-release-loomwatch-data
```

## Parameters

### Global parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `global.imageRegistry` | Global Docker image registry | `""` |
| `global.imagePullSecrets` | Global Docker registry secret names as an array | `[]` |
| `global.storageClass` | Global StorageClass for Persistent Volume(s) | `""` |
| `global.defaultStorageClass` | Global default StorageClass for Persistent Volume(s) | `""` |

### Common parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `kubeVersion` | Override Kubernetes version | `""` |
| `nameOverride` | String to partially override loomwatch.fullname | `""` |
| `fullnameOverride` | String to fully override loomwatch.fullname | `""` |
| `namespaceOverride` | String to fully override the deployment namespace | `""` |
| `commonLabels` | Labels to add to all deployed objects | `{}` |
| `commonAnnotations` | Annotations to add to all deployed objects | `{}` |
| `clusterDomain` | Kubernetes cluster domain name | `cluster.local` |
| `extraDeploy` | Array of extra objects to deploy with the release | `[]` |
| `diagnosticMode.enabled` | Enable diagnostic mode (all probes will be disabled and the command will be overridden) | `false` |
| `diagnosticMode.command` | Command to override all containers in the deployment | `['sleep']` |
| `diagnosticMode.args` | Args to override all containers in the deployment | `['infinity']` |

### loomwatch Image parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `image.registry` | loomwatch image registry | `ghcr.io` |
| `image.repository` | loomwatch image repository | `concordloom/loomwatch` |
| `image.tag` | loomwatch image tag; empty means the chart's appVersion, which is the version this chart ships | `""` |
| `image.digest` | loomwatch image digest in the way sha256:aa.... Please note this parameter, if set, will override the tag | `""` |
| `image.pullPolicy` | loomwatch image pull policy | `IfNotPresent` |
| `image.pullSecrets` | loomwatch image pull secrets | `[]` |

### loomwatch configuration parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `containerPort` | loomwatch HTTP container port | `9211` |
| `logLevel` | loomwatch log level. Allowed values: `debug`, `info`, `warn`, `error` | `info` |
| `pollInterval` | Seconds between provider quota polls | `120` |
| `databasePath` | Path of the SQLite database inside the container | `/data/onwatch.db` |
| `extraEnvVars` | Array with extra environment variables to add to the loomwatch container | `[]` |
| `extraEnvVarsCM` | Name of existing ConfigMap containing extra env vars | `""` |
| `extraEnvVarsSecret` | Name of existing Secret containing extra env vars | `""` |

### Credentials parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `auth.adminPassword` | Password for the built-in `admin` panel user. Generated randomly if empty and no existing secret is given | `""` |
| `auth.metricsToken` | Bearer token required on /metrics. Generated randomly if empty and no existing secret is given | `""` |
| `auth.providers` | Map of provider credential environment variables to their values | `{}` |
| `auth.accounts` | Provider subscriptions declared in configuration rather than created in the dashboard | `[]` |
| `auth.existingSecret` | Name of an existing Secret holding all credentials. Takes precedence over the values above | `""` |
| `auth.secretKeys.adminPasswordKey` | Key in the existing Secret holding the admin password | `ONWATCH_ADMIN_PASS` |
| `auth.secretKeys.metricsTokenKey` | Key in the existing Secret holding the metrics token | `ONWATCH_METRICS_TOKEN` |

### loomwatch deployment parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `replicaCount` | Number of loomwatch replicas to deploy | `1` |
| `updateStrategy.type` | loomwatch deployment strategy type | `Recreate` |
| `priorityClassName` | loomwatch pod priority class name | `""` |
| `schedulerName` | Name of the Kubernetes scheduler (other than default) | `""` |
| `terminationGracePeriodSeconds` | Seconds loomwatch pod needs to terminate gracefully | `""` |
| `topologySpreadConstraints` | Topology Spread Constraints for pod assignment | `[]` |
| `podLabels` | Extra labels for loomwatch pods | `{}` |
| `podAnnotations` | Annotations for loomwatch pods | `{}` |
| `podAffinityPreset` | Pod affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard` | `""` |
| `podAntiAffinityPreset` | Pod anti-affinity preset. Ignored if `affinity` is set. Allowed values: `soft` or `hard` | `soft` |
| `nodeAffinityPreset.type` | Node affinity preset type. Ignored if `affinity` is set. Allowed values: `soft` or `hard` | `""` |
| `nodeAffinityPreset.key` | Node label key to match. Ignored if `affinity` is set | `""` |
| `nodeAffinityPreset.values` | Node label values to match. Ignored if `affinity` is set | `[]` |
| `affinity` | Affinity for pod assignment | `{}` |
| `nodeSelector` | Node labels for pod assignment | `{}` |
| `tolerations` | Tolerations for pod assignment | `[]` |
| `podSecurityContext.enabled` | Enabled loomwatch pods' Security Context | `true` |
| `podSecurityContext.fsGroupChangePolicy` | Set filesystem group change policy | `Always` |
| `podSecurityContext.supplementalGroups` | Set filesystem extra groups | `[]` |
| `podSecurityContext.fsGroup` | Set loomwatch pod's Security Context fsGroup | `65532` |
| `containerSecurityContext.enabled` | Enabled loomwatch containers' Security Context | `true` |
| `containerSecurityContext.seLinuxOptions` | Set SELinux options in container | `{}` |
| `containerSecurityContext.runAsUser` | Set loomwatch containers' Security Context runAsUser | `65532` |
| `containerSecurityContext.runAsGroup` | Set loomwatch containers' Security Context runAsGroup | `65532` |
| `containerSecurityContext.runAsNonRoot` | Set loomwatch containers' Security Context runAsNonRoot | `true` |
| `containerSecurityContext.readOnlyRootFilesystem` | Set loomwatch containers' Security Context readOnlyRootFilesystem | `true` |
| `containerSecurityContext.privileged` | Set loomwatch containers' Security Context privileged | `false` |
| `containerSecurityContext.allowPrivilegeEscalation` | Set loomwatch containers' Security Context allowPrivilegeEscalation | `false` |
| `containerSecurityContext.capabilities.drop` | List of capabilities to be dropped | `['ALL']` |
| `containerSecurityContext.seccompProfile.type` | Set container's Security Context seccomp profile | `RuntimeDefault` |
| `resourcesPreset` | Set container resources according to one common preset. Ignored if `resources` is set | `nano` |
| `resources` | Set container requests and limits for different resources like CPU or memory | `{}` |
| `livenessProbe.enabled` | Enable livenessProbe on loomwatch containers | `true` |
| `livenessProbe.initialDelaySeconds` | Initial delay seconds for livenessProbe | `10` |
| `livenessProbe.periodSeconds` | Period seconds for livenessProbe | `20` |
| `livenessProbe.timeoutSeconds` | Timeout seconds for livenessProbe | `5` |
| `livenessProbe.failureThreshold` | Failure threshold for livenessProbe | `6` |
| `livenessProbe.successThreshold` | Success threshold for livenessProbe | `1` |
| `readinessProbe.enabled` | Enable readinessProbe on loomwatch containers | `true` |
| `readinessProbe.initialDelaySeconds` | Initial delay seconds for readinessProbe | `5` |
| `readinessProbe.periodSeconds` | Period seconds for readinessProbe | `10` |
| `readinessProbe.timeoutSeconds` | Timeout seconds for readinessProbe | `5` |
| `readinessProbe.failureThreshold` | Failure threshold for readinessProbe | `6` |
| `readinessProbe.successThreshold` | Success threshold for readinessProbe | `1` |
| `startupProbe.enabled` | Enable startupProbe on loomwatch containers | `false` |
| `startupProbe.initialDelaySeconds` | Initial delay seconds for startupProbe | `5` |
| `startupProbe.periodSeconds` | Period seconds for startupProbe | `10` |
| `startupProbe.timeoutSeconds` | Timeout seconds for startupProbe | `5` |
| `startupProbe.failureThreshold` | Failure threshold for startupProbe | `30` |
| `startupProbe.successThreshold` | Success threshold for startupProbe | `1` |
| `customLivenessProbe` | Custom livenessProbe that overrides the default one | `{}` |
| `customReadinessProbe` | Custom readinessProbe that overrides the default one | `{}` |
| `customStartupProbe` | Custom startupProbe that overrides the default one | `{}` |
| `lifecycleHooks` | for the loomwatch container(s) to automate configuration before or after startup | `{}` |
| `command` | Override default container command (useful when using custom images) | `[]` |
| `args` | Override default container args (useful when using custom images) | `[]` |
| `automountServiceAccountToken` | Mount Service Account token in loomwatch pods | `false` |
| `hostAliases` | loomwatch pods host aliases | `[]` |
| `extraVolumes` | Optionally specify extra list of additional volumes for the loomwatch pod(s) | `[]` |
| `extraVolumeMounts` | Optionally specify extra list of additional volumeMounts for the loomwatch container(s) | `[]` |
| `sidecars` | Add additional sidecar containers to the loomwatch pod(s) | `[]` |
| `initContainers` | Add additional init containers to the loomwatch pod(s) | `[]` |

### Traffic Exposure Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `service.type` | loomwatch service type | `ClusterIP` |
| `service.ports.http` | loomwatch service HTTP port | `9211` |
| `service.nodePorts.http` | Node port for HTTP | `""` |
| `service.clusterIP` | loomwatch service Cluster IP | `""` |
| `service.loadBalancerIP` | loomwatch service Load Balancer IP | `""` |
| `service.loadBalancerSourceRanges` | loomwatch service Load Balancer sources | `[]` |
| `service.externalTrafficPolicy` | loomwatch service external traffic policy | `Cluster` |
| `service.sessionAffinity` | Control where client requests go, to the same pod or round-robin | `None` |
| `service.annotations` | Additional custom annotations for loomwatch service | `{}` |
| `service.extraPorts` | Extra ports to expose in loomwatch service (normally used with the `sidecars` value) | `[]` |
| `ingress.enabled` | Enable ingress record generation for loomwatch | `false` |
| `ingress.pathType` | Ingress path type | `ImplementationSpecific` |
| `ingress.apiVersion` | Force Ingress API version (automatically detected if not set) | `""` |
| `ingress.hostname` | Default host for the ingress record | `loomwatch.local` |
| `ingress.ingressClassName` | IngressClass that will be used to implement the Ingress | `""` |
| `ingress.path` | Default path for the ingress record | `/` |
| `ingress.annotations` | Additional annotations for the Ingress resource | `{}` |
| `ingress.tls` | Enable TLS configuration for the host defined at `ingress.hostname` parameter | `false` |
| `ingress.selfSigned` | Create a TLS secret for this ingress record using self-signed certificates generated by Helm | `false` |
| `ingress.extraHosts` | An array with additional hostname(s) to be covered with the ingress record | `[]` |
| `ingress.extraPaths` | An array with additional arbitrary paths that may need to be added to the ingress under the main host | `[]` |
| `ingress.extraTls` | TLS configuration for additional hostname(s) to be covered with this ingress record | `[]` |
| `ingress.secrets` | Custom TLS certificates as secrets | `[]` |
| `ingress.extraRules` | Additional rules to be covered with this ingress record | `[]` |
| `httpRoute.enabled` | Enable Gateway API HTTPRoute record generation | `false` |
| `httpRoute.apiVersion` | HTTPRoute API version | `gateway.networking.k8s.io/v1` |
| `httpRoute.parentRefs` | Gateways this route attaches to. Required when enabled | `[]` |
| `httpRoute.hostnames` | Hostnames this route matches. Empty means every hostname the Gateway serves | `[]` |
| `httpRoute.path` | Path prefix for the default rule | `/` |
| `httpRoute.pathType` | Path match type for the default rule. Allowed values: `PathPrefix`, `Exact`, `RegularExpression` | `PathPrefix` |
| `httpRoute.filters` | Filters applied to the default rule | `[]` |
| `httpRoute.weight` | Backend weight for the default rule | `""` |
| `httpRoute.rules` | Replace the generated rule entirely with your own | `[]` |
| `httpRoute.annotations` | Additional annotations for the HTTPRoute resource | `{}` |
| `httpRoute.labels` | Additional labels for the HTTPRoute resource | `{}` |
| `httpRoute.allowAlongsideIngress` | Permit an Ingress and an HTTPRoute at the same time | `false` |

### Persistence Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `persistence.enabled` | Enable persistence using Persistent Volume Claims | `true` |
| `persistence.mountPath` | Path to mount the volume at | `/data` |
| `persistence.subPath` | The subdirectory of the volume to mount to | `""` |
| `persistence.storageClass` | Storage class of backing PVC | `""` |
| `persistence.annotations` | Persistent Volume Claim annotations | `{}` |
| `persistence.labels` | Persistent Volume Claim labels | `{}` |
| `persistence.accessModes` | Persistent Volume Access Modes | `['ReadWriteOnce']` |
| `persistence.size` | Size of data volume | `1Gi` |
| `persistence.selector` | Selector to match an existing Persistent Volume for the data PVC | `{}` |
| `persistence.dataSource` | Custom PVC data source | `{}` |
| `persistence.existingClaim` | The name of an existing PVC to use for persistence | `""` |

### Other Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `serviceAccount.create` | Enable creation of ServiceAccount for loomwatch pod | `true` |
| `serviceAccount.name` | The name of the ServiceAccount to use | `""` |
| `serviceAccount.annotations` | Additional Service Account annotations | `{}` |
| `serviceAccount.automountServiceAccountToken` | Automount service account token for the deployment controller service account | `false` |
| `pdb.create` | Enable/disable a Pod Disruption Budget creation | `false` |
| `pdb.minAvailable` | Minimum number/percentage of pods that should remain scheduled | `""` |
| `pdb.maxUnavailable` | Maximum number/percentage of pods that may be made unavailable | `""` |
| `networkPolicy.enabled` | Specifies whether a NetworkPolicy should be created | `true` |
| `networkPolicy.allowExternal` | Don't require server label for connections | `true` |
| `networkPolicy.allowExternalEgress` | Allow the pod to access any range of port and all destinations | `true` |
| `networkPolicy.extraIngress` | Add extra ingress rules to the NetworkPolicy | `[]` |
| `networkPolicy.extraEgress` | Add extra egress rules to the NetworkPolicy | `[]` |
| `networkPolicy.ingressPodMatchLabels` | Labels to match to allow traffic from other pods | `{}` |
| `networkPolicy.ingressNSMatchLabels` | Labels to match to allow traffic from other namespaces | `{}` |
| `networkPolicy.ingressNSPodMatchLabels` | Pod labels to match to allow traffic from other namespaces | `{}` |

### Metrics Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `metrics.enabled` | Protect the /metrics endpoint with a bearer token | `true` |
| `metrics.serviceMonitor.enabled` | Create a ServiceMonitor resource for scraping metrics using Prometheus Operator | `false` |
| `metrics.serviceMonitor.namespace` | Namespace in which the ServiceMonitor will be created | `""` |
| `metrics.serviceMonitor.interval` | Scrape interval. If not set, the Prometheus default scrape interval is used | `60s` |
| `metrics.serviceMonitor.scrapeTimeout` | Timeout after which the scrape is ended | `""` |
| `metrics.serviceMonitor.labels` | Additional labels for the ServiceMonitor. Usually needed for the Prometheus Operator to pick it up | `{}` |
| `metrics.serviceMonitor.annotations` | Additional annotations for the ServiceMonitor | `{}` |
| `metrics.serviceMonitor.honorLabels` | Specify honorLabels parameter to add the scrape endpoint | `false` |
| `metrics.serviceMonitor.relabelings` | Metric relabelings to apply to samples before ingestion | `[]` |
| `metrics.serviceMonitor.metricRelabelings` | Metrics relabelings to apply to samples before ingestion | `[]` |
| `metrics.serviceMonitor.jobLabel` | The name of the label on the target service to use as the job name | `""` |
| `metrics.serviceMonitor.selector` | Prometheus instance selector labels | `{}` |
| `metrics.prometheusRule.enabled` | Create a PrometheusRule resource for alerting on quota exhaustion | `false` |
| `metrics.prometheusRule.namespace` | Namespace in which the PrometheusRule will be created | `""` |
| `metrics.prometheusRule.labels` | Additional labels for the PrometheusRule | `{}` |
| `metrics.prometheusRule.annotations` | Additional annotations for the PrometheusRule | `{}` |
| `metrics.prometheusRule.providers` | Regex of providers the default rules observe | `.+` |
| `metrics.prometheusRule.ignoredQuotaTypes` | Regex of quota types excluded from the default rules | `.*video` |
| `metrics.prometheusRule.teams` | Map provider accounts to the team that owns them | `[]` |
| `metrics.prometheusRule.nonAccountProviders` | Regex of providers excluded from team ownership | `api_integrations` |
| `metrics.prometheusRule.runbookUrlBase` | Base URL for the runbook_url annotation on each alert | `https://github.com/concordloom/loomwatch/blob/main/docs/runbooks/README.md` |
| `metrics.prometheusRule.highThreshold` | Utilisation percentage above which LoomwatchQuotaHigh fires | `80` |
| `metrics.prometheusRule.criticalThreshold` | Utilisation percentage above which LoomwatchQuotaCritical fires | `95` |
| `metrics.prometheusRule.burn.trendWindow` | Range over which the burn slope is measured | `24h` |
| `metrics.prometheusRule.burn.maxHorizonSeconds` | Do not predict for windows resetting further away than this | `172800` |
| `metrics.prometheusRule.defaultRules.quotaHigh` | Enable the LoomwatchQuotaHigh rule | `true` |
| `metrics.prometheusRule.defaultRules.quotaCritical` | Enable the LoomwatchQuotaCritical rule | `true` |
| `metrics.prometheusRule.defaultRules.burnRate` | Enable the LoomwatchQuotaBurnsBeforeReset rule | `true` |
| `metrics.prometheusRule.defaultRules.collectorNotPolling` | Enable the LoomwatchCollectorNotPolling rule | `true` |
| `metrics.prometheusRule.defaultRules.collectorStale` | Enable the LoomwatchCollectorStale rule | `true` |
| `metrics.prometheusRule.defaultRules.accountWithoutTeam` | Enable the LoomwatchAccountWithoutTeam rule (only rendered when teams are configured) | `true` |
| `metrics.prometheusRule.extraRules` | Additional rules appended to the generated group | `[]` |
| `dashboard.enabled` | Ship the quota dashboard as a ConfigMap for Grafana to import, through a sidecar or through grafana-operator | `false` |
| `dashboard.namespace` | Namespace in which the dashboard ConfigMap will be created | `""` |
| `dashboard.labels` | Labels the Grafana dashboard sidecar selects on | `{'grafana_dashboard': '1'}` |
| `dashboard.annotations` | Additional annotations for the dashboard ConfigMap | `{}` |

## Configuration and installation details

### What the metrics mean

```
loomwatch_quota_utilization_percent{provider,quota_type,account_id}
loomwatch_quota_reset_timestamp_seconds{provider,quota_type,account_id}
loomwatch_agent_healthy{provider,account_id}
loomwatch_agent_last_cycle_age_seconds{provider,account_id}
```

Two properties are worth stating because alerts depend on them:

- **A series exists when the provider declares the quota, not when it has been
  consumed.** Zero utilisation is a real reading. An absent series means the
  plan has no such quota — not that it is idle.
- **`quota_type` is the window, and its length is not implied by its name.** A
  provider may reset `general` every five hours and `tokens` weekly. Group by
  the reset timestamp, never by the name.

### Getting the dashboard into Grafana

`dashboard.enabled=true` renders the quota dashboard into a ConfigMap named
`<fullname>-dashboard`, under the key `loomwatch.json`. How Grafana picks it up
depends on what is installed next to it, and the chart assumes neither.

**A dashboard sidecar** watches ConfigMaps carrying a label - `grafana_dashboard:
"1"` by default, which is what `dashboard.labels` already sets. Nothing further
is needed.

**grafana-operator** does not read labelled ConfigMaps at all. It imports what a
`GrafanaDashboard` resource points at, so keep the ConfigMap and add the resource
through `extraDeploy`. The dashboard JSON then stays inside the chart instead of
being copied into your values, where it would drift the first time the chart is
upgraded:

```yaml
dashboard:
  enabled: true

extraDeploy:
  - apiVersion: grafana.integreatly.org/v1beta1
    kind: GrafanaDashboard
    metadata:
      name: loomwatch-quotas
    spec:
      instanceSelector:
        matchLabels:
          dashboards: grafana
      configMapRef:
        name: loomwatch-dashboard
        key: loomwatch.json
```

`configMapRef.name` is the release's fullname plus `-dashboard`, so it follows
`fullnameOverride` if you set one. The operator re-reads the ConfigMap on its
resync period, ten minutes by default, so a chart upgrade shows up in Grafana
without touching the resource.

### Alerting rules

`metrics.prometheusRule.enabled=true` installs five rules. The two threshold
rules are shares of the plan's own limit. The burn rule answers a different
question — *will this run out before the window resets* — and two things about
it are deliberate:

It is written as `current + slope * seconds_remaining` rather than with
`predict_linear`. That function takes a **scalar** horizon and so cannot use each
series' own reset time; passing the reset vector to it is a parse error, and the
resulting rule never evaluates while still reporting healthy in some UIs.

It only predicts when the window resets within
`metrics.prometheusRule.burn.maxHorizonSeconds`. Extrapolating a one-day trend
across a week is noise amplified sevenfold — one batch job projects a breach that
will never happen. Beyond the gate the rule stays silent rather than guessing.

### Excluding quotas from alerts

`metrics.prometheusRule.ignoredQuotaTypes` defaults to `.*video`, and the leading
`.*` is load-bearing. PromQL label matchers compare the **whole** string, so
`video` would not exclude `weekly_video`.

### Credentials

Providers are a free-form map because upstream supports more than ten and the
set changes between releases:

```yaml
auth:
  providers:
    MINIMAX_API_KEY: "..."
    ZAI_API_KEY: "..."
```

To keep secrets out of values, create the Secret yourself and point the chart at
it. It is consumed with `envFrom`, so every key becomes an environment variable:

```console
kubectl create secret generic loomwatch-credentials \
  --from-literal=ONWATCH_ADMIN_PASS=... \
  --from-literal=ONWATCH_METRICS_TOKEN=... \
  --from-literal=MINIMAX_API_KEY=...
```

```yaml
auth:
  existingSecret: loomwatch-credentials
```

When left empty, the admin password and metrics token are generated and then
**read back from the existing Secret on upgrade**, so they survive `helm
upgrade`. Without that, every upgrade would rotate the token and quietly turn
every Prometheus scrape into a 401.

Some providers only allow adding a second account through the dashboard. Those
accounts live in the database, which is why persistence is enabled by default.

### Reaching providers through a proxy

Whether a provider is reachable is a property of your network, not of loomwatch,
so there is no dedicated setting. Use `sidecars` and `extraEnvVars`:

```yaml
sidecars:
  - name: proxy
    image: your/proxy:tag
extraEnvVars:
  - name: HTTPS_PROXY
    value: http://127.0.0.1:7890
  - name: NO_PROXY
    value: localhost,127.0.0.1,.svc,.svc.cluster.local
```

### Why a single replica

The collector is a single writer against a SQLite file on a ReadWriteOnce
volume. A second replica cannot attach the same volume, and without persistence
it would instead double every poll against quotas that are themselves rate
limited. The chart refuses `replicaCount > 1` rather than letting you find out
in production.

For the same reason the update strategy is `Recreate`: with a ReadWriteOnce
volume, `RollingUpdate` schedules the new Pod before the old one releases the
volume, and if it lands on another node the rollout deadlocks on a Multi-Attach
error and never completes.

### Exposing the dashboard

Two routers are supported and neither is on by default: `ingress.*` for
Ingress, `httpRoute.*` for Gateway API. Which one publishes a service is a
decision, so the chart does not pick based on which CRDs happen to be installed
— that would give different results on two clusters from identical values.

Rendering both at once is refused unless `httpRoute.allowAlongsideIngress` says
otherwise: two routes to one Service is nearly always a migration someone
stopped halfway through, and the forgotten one keeps serving on an address
nobody watches.

`httpRoute.parentRefs` is required when enabled. A route with no parent is
accepted by the API server and then routes nothing, which reads as a successful
install until someone opens the address.

The panel carries the whole surface behind one password with no second factor,
so `ingress.enabled` is `false` by default. The metrics endpoint is a better
integration point in most cases, and it is what the alerts use.

## Differences from upstream

- **The weekly quota window is exported.** Upstream reads it from its own store
  but never publishes it, so an account could sit at 80% of its week with every
  quota rule silent.
- **A quota series is gated on being declared, not on being consumed.** Upstream
  gated on usage, which hid the series exactly while the account was healthy and
  hid it permanently for plans reporting a percentage with zero usage
  ([upstream #112](https://github.com/onllm-dev/onWatch/issues/112)).
- **No self-update.** It could not work here - the releases carry no assets and
  the container is read-only and distroless - so the dashboard no longer offers
  it. The unit of delivery is the image.
- **This chart**, with alerting rules and a dashboard.

## Troubleshooting

**No quota series at all.** The collector has no credentials, or has not polled
yet. Polls happen every `pollInterval` seconds; check the container logs for
`poll complete`.

**Prometheus scrapes return 401.** The ServiceMonitor reads the bearer token from
the Secret. If you replaced the Secret without rolling the Pod, the running
process still holds the old token.

**Rollout stuck in `ContainerCreating` with `Multi-Attach error`.** The update
strategy was changed to `RollingUpdate` while the volume is ReadWriteOnce. Set
it back to `Recreate`.

**Pod exits with "database path is not writable".** `podSecurityContext.fsGroup`
does not match the image's user, or `databasePath` points outside the mounted
volume.

## License

GPL-3.0-only, inherited from upstream onWatch.

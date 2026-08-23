# Listing the chart where people look for charts

Two catalogues, both free, both requiring a person with an account to click
once. Everything that can be prepared in the repository already is.

## Artifact Hub

The catalogue people search when they do not already know a chart exists. The
repository is not listed there today, and the query `llm quota` returns nothing
at all, so the category is empty rather than crowded.

1. Sign in at <https://artifacthub.io> with GitHub.
2. Control Panel, Repositories, Add.
   - Kind: **Helm charts**
   - Name: `loomwatch`
   - URL: `oci://ghcr.io/concordloom/charts/loomwatch`

   OCI repositories are listed per chart rather than per registry, because an
   OCI registry has no `index.yaml` to enumerate.
3. Artifact Hub returns a **repository ID**. Put it in `artifacthub-repo.yml`
   at the repository root and push that artifact to the registry to claim
   ownership - that is what turns on the verified-publisher badge. The exact
   media type for OCI has changed more than once; take it from Artifact Hub's
   own documentation at the time you do this rather than from here.

What the page will show is already in `Chart.yaml`: category, license, links to
the chart README, the runbooks and the metrics reference, and the image list it
security-scans. That last one is checked against `appVersion` on every pull
request, because nothing renders it and it had drifted eight minor versions
before anyone noticed.

## Grafana dashboard catalogue

<https://grafana.com/grafana/dashboards> is a second entrance, and it works the
other way round: people arrive looking for a dashboard and leave with the chart.

1. Sign in at <https://grafana.com>.
2. Dashboards, New, Upload, and give it
   `charts/loomwatch/dashboards/loomwatch.json`.
3. The listing gets a numeric ID. Put it in the chart README so the dashboard
   can be imported by number without cloning anything.

The dashboard is already shaped for this: the datasource is a template variable
rather than a hard-coded UID, so it imports into any Prometheus without editing.

## What is deliberately not here

No landing page, and no search-engine work. Platform engineers do not find
exporters through a search engine, and a landing page converts traffic that does
not exist yet. The chart README and the Artifact Hub page are the landing page.

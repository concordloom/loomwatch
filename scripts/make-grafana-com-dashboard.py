#!/usr/bin/env python3
"""Derive the grafana.com catalogue variant of the dashboard.

Two consumers want two different files and neither can use the other's.

The chart provisions its dashboard into a Grafana that already has a datasource,
so it selects one through a `datasource` template variable. The grafana.com
catalogue imports into an unknown Grafana and asks the person doing the import
which datasource to use, which means the "export for sharing externally"
envelope: `__inputs`, `__requires`, and `${DS_PROMETHEUS}` in place of the
variable. Ship the catalogue form in the chart and provisioning breaks on an
unresolved placeholder; ship the chart form to the catalogue and it refuses the
upload as an old JSON format.

So the catalogue file is generated from the chart file rather than maintained
beside it, and --check fails the build when the two have drifted.

  scripts/make-grafana-com-dashboard.py [--check]
"""
import copy
import json
import sys

SOURCE = "charts/loomwatch/dashboards/loomwatch.json"
TARGET = "dashboards/grafana-com/loomwatch.json"

# Panel types the dashboard uses, named the way Grafana names them.
PANEL_NAMES = {"timeseries": "Time series", "stat": "Stat"}

# The oldest Grafana this dashboard is known to import into. schemaVersion 39
# corresponds to the 10.x line.
MIN_GRAFANA = "10.0.0"


def derive(src):
    d = copy.deepcopy(src)

    # The datasource variable becomes an import-time input.
    variables = d.get("templating", {}).get("list", [])
    ds_vars = [v["name"] for v in variables if v.get("type") == "datasource"]
    d["templating"]["list"] = [v for v in variables if v.get("type") != "datasource"]

    blob = json.dumps(d)
    for name in ds_vars:
        blob = blob.replace("${%s}" % name, "${DS_PROMETHEUS}")
    d = json.loads(blob)

    panel_types = sorted({p["type"] for p in d["panels"]})
    unknown = [t for t in panel_types if t not in PANEL_NAMES]
    if unknown:
        raise SystemExit(
            f"unknown panel types {unknown}: add them to PANEL_NAMES so "
            f"__requires stays truthful"
        )

    out = {
        "__inputs": [
            {
                "name": "DS_PROMETHEUS",
                "label": "Prometheus",
                "description": "The Prometheus that scrapes loomwatch.",
                "type": "datasource",
                "pluginId": "prometheus",
                "pluginName": "Prometheus",
            }
        ],
        "__requires": [
            {
                "type": "grafana",
                "id": "grafana",
                "name": "Grafana",
                "version": MIN_GRAFANA,
            },
            {
                "type": "datasource",
                "id": "prometheus",
                "name": "Prometheus",
                "version": "1.0.0",
            },
        ]
        + [
            {"type": "panel", "id": t, "name": PANEL_NAMES[t], "version": ""}
            for t in panel_types
        ],
        "id": None,
    }
    # uid is assigned by the importing Grafana, not by the catalogue.
    d.pop("uid", None)
    out.update(d)
    return out


def main(check):
    src = json.load(open(SOURCE))
    want = json.dumps(derive(src), indent=2, ensure_ascii=False) + "\n"

    if check:
        try:
            have = open(TARGET).read()
        except FileNotFoundError:
            print(f"{TARGET} is missing; run scripts/make-grafana-com-dashboard.py")
            return 1
        if have != want:
            print(f"{TARGET} is stale; run scripts/make-grafana-com-dashboard.py")
            return 1
        print(f"{TARGET} is up to date")
        return 0

    open(TARGET, "w").write(want)
    print(f"{TARGET} written")
    return 0


if __name__ == "__main__":
    sys.exit(main("--check" in sys.argv))

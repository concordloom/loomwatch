#!/usr/bin/env python3
"""Generate the loomwatch Grafana dashboard.

The dashboard is generated rather than hand-edited because its shape is
repetitive and its correctness is in the queries, which are long and appear in
several panels with small deliberate differences. A JSON file of this size
edited by hand drifts: a selector gets fixed in one panel and not the next, and
the panel that still has the old one looks like data rather than a bug.

Two questions, two shapes:

  "What is running out?" is a sort, not a curve. It gets a table, ordered by
  what breaches first, so the answer is the top row rather than a hunt through
  sixteen overlapping lines.

  "What is going on with this subscription?" is per provider. It gets a row
  that repeats over the provider variable, so adding a subscription adds a
  block instead of adding a line to an already unreadable legend.

Ownership is a column, never a panel. A panel that is empty until someone
configures a mapping is a hole in the layout for everyone who has not.
"""
import json

DS = {"type": "prometheus", "uid": "${datasource}"}
SEL = 'provider=~"$provider",quota_type=~"$quota_type"'

# Utilisation is a share of each plan's own limit, so these are absolute.
STEPS = [
    {"color": "green", "value": None},
    {"color": "orange", "value": 80},
    {"color": "red", "value": 95},
]


def target(ref, expr, legend=None, instant=False):
    t = {"refId": ref, "datasource": DS, "expr": " ".join(expr.split())}
    if instant:
        t.update({"instant": True, "range": False, "format": "table"})
    if legend:
        t["legendFormat"] = legend
    return t


def projection(sel):
    """Current utilisation plus its own slope to its own reset time.

    Written as vector arithmetic rather than predict_linear because that takes
    a scalar horizon and cannot use each series' own reset timestamp.
    """
    return f"""
      max by (provider, quota_type, account_id) (loomwatch_quota_utilization_percent{{{sel}}})
      + max by (provider, quota_type, account_id) (deriv(loomwatch_quota_utilization_percent{{{sel}}}[$__range]))
        * max by (provider, quota_type, account_id) (loomwatch_quota_reset_timestamp_seconds{{{sel}}} - time())
    """


def triage_table():
    return {
        "id": 1,
        "type": "table",
        "title": "What runs out first",
        "description": (
            "Every quota, ordered by what breaches first. This is the panel for "
            "the person who was just paged: the answer is the top row. "
            "Utilisation is a share of each plan's own limit, so 100 is the "
            "quota itself - and for the same reason these numbers cannot be "
            "summed. The team column appears only where ownership is mapped in "
            "the chart."
        ),
        "datasource": DS,
        "gridPos": {"h": 15, "w": 24, "x": 0, "y": 0},
        "targets": [
            target("A", f"max by (provider, quota_type, account_id) (loomwatch_quota_utilization_percent{{{SEL}}})", instant=True),
            target("B", f"max by (provider, quota_type, account_id) (loomwatch_quota_reset_timestamp_seconds{{{SEL}}} - time())", instant=True),
            target("C", projection(SEL), instant=True),
            target("D", f"""
                max by (provider, quota_type, account_id, team) (
                  loomwatch_quota_utilization_percent{{{SEL}}}
                  * on (provider, account_id) group_left(team) loomwatch:account_team{{team=~"$team"}}
                ) * 0
            """, instant=True),
        ],
        "transformations": [
            {"id": "merge", "options": {}},
            {"id": "organize", "options": {
                "excludeByName": {"Time": True, "Value #D": True},
                "renameByName": {
                    "provider": "Provider",
                    "account_id": "Account",
                    "quota_type": "Window",
                    "team": "Team",
                    "Value #A": "Utilisation",
                    "Value #B": "Resets in",
                    "Value #C": "Projected at reset",
                },
                "indexByName": {
                    "provider": 0, "account_id": 1, "quota_type": 2, "team": 3,
                    "Value #A": 4, "Value #B": 5, "Value #C": 6,
                },
            }},
        ],
        "fieldConfig": {
            "defaults": {"custom": {"align": "auto", "cellOptions": {"type": "auto"}}},
            "overrides": [
                {"matcher": {"id": "byName", "options": "Utilisation"},
                 "properties": [
                     {"id": "unit", "value": "percent"},
                     {"id": "min", "value": 0}, {"id": "max", "value": 100},
                     {"id": "custom.cellOptions", "value": {"type": "gauge", "mode": "gradient"}},
                     {"id": "thresholds", "value": {"mode": "absolute", "steps": STEPS}},
                 ]},
                {"matcher": {"id": "byName", "options": "Resets in"},
                 "properties": [{"id": "unit", "value": "s"}]},
                {"matcher": {"id": "byName", "options": "Projected at reset"},
                 "properties": [
                     {"id": "unit", "value": "percent"}, {"id": "decimals", "value": 0},
                     {"id": "custom.cellOptions", "value": {"type": "color-text"}},
                     {"id": "thresholds", "value": {"mode": "absolute", "steps": STEPS}},
                 ]},
                {"matcher": {"id": "byName", "options": "Provider"}, "properties": [{"id": "custom.width", "value": 120}]},
                {"matcher": {"id": "byName", "options": "Account"}, "properties": [{"id": "custom.width", "value": 90}]},
                {"matcher": {"id": "byName", "options": "Window"}, "properties": [{"id": "custom.width", "value": 150}]},
            ],
        },
        "options": {
            "showHeader": True, "footer": {"show": False}, "cellHeight": "sm",
            # The table's own sort rather than a sortBy transformation: the
            # transformation matches on the field name at its point in the
            # chain, which is before the rename, and a name it cannot find is
            # not an error - it is a table that quietly comes back unsorted.
            #
            # Utilisation is the sort key, not the projection. It is defined for
            # every row, while a projection needs a reset timestamp and several
            # windows do not publish one. Sorting by a column that is empty for
            # half the rows puts the empties somewhere arbitrary and buries the
            # full quota under them.
            "sortBy": [{"displayName": "Utilisation", "desc": True}],
        },
    }


def account_row():
    """A row that repeats over $account: one block per subscription.

    Per account rather than per provider because an account is the thing
    somebody pays for and adds one at a time; a provider is a category. Three
    Z.ai accounts sharing one block are three accounts the reader has to
    separate by squinting at bar labels.
    """
    return {
        "id": 10,
        "type": "row",
        "title": "$account",
        "collapsed": False,
        "repeat": "account",
        "gridPos": {"h": 1, "w": 24, "x": 0, "y": 15},
        "panels": [],
    }


def per_account_panels():
    # account_id alone is enough to name an account: provider_accounts.id is a
    # single autoincrement shared by every provider, so an id belongs to exactly
    # one of them. The exception is providers that keep no account rows at all -
    # they all report "default" and therefore share one block, which is visible
    # rather than silent.
    psel = 'account_id=~"$account",quota_type=~"$quota_type"' 
    return [
        {
            "id": 11,
            "type": "bargauge",
            "title": "Quotas now",
            "description": "One bar per account and quota window, labelled. Bars rather than lines because the question here is how full, not how it got there.",
            "datasource": DS,
            "gridPos": {"h": 9, "w": 9, "x": 0, "y": 16},
            "targets": [target("A", f"max by (quota_type) (loomwatch_quota_utilization_percent{{{psel}}})",
                               legend="{{quota_type}}")],
            "fieldConfig": {
                "defaults": {
                    "unit": "percent", "min": 0, "max": 100,
                    "thresholds": {"mode": "absolute", "steps": STEPS},
                },
                "overrides": [],
            },
            "options": {
                "orientation": "horizontal",
                "displayMode": "gradient",
                # Fixed sizes: a bar gauge with three bars stretches its labels
                # to headline size, and the same panel next to an account with
                # eight bars then reads as a different kind of thing.
                "text": {"titleSize": 14, "valueSize": 28},
                "showUnfilled": True,
                "valueMode": "text",
                "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
            },
        },
        {
            "id": 12,
            "type": "timeseries",
            "title": "Trend",
            "description": "Lines, not fills: sixteen filled areas are mud. The legend is a sorted table on the right so a series can be found by name rather than by colour.",
            "datasource": DS,
            "gridPos": {"h": 9, "w": 10, "x": 9, "y": 16},
            "targets": [target("A", f"max by (quota_type) (loomwatch_quota_utilization_percent{{{psel}}})",
                               legend="{{quota_type}}")],
            "fieldConfig": {
                "defaults": {
                    "unit": "percent", "min": 0, "max": 100,
                    "custom": {"lineWidth": 2, "fillOpacity": 0, "showPoints": "never"},
                    "thresholds": {"mode": "absolute", "steps": STEPS},
                },
                "overrides": [],
            },
            "options": {
                "legend": {"displayMode": "table", "placement": "right",
                           "calcs": ["lastNotNull"], "sortBy": "Last *", "sortDesc": True},
                "tooltip": {"mode": "multi", "sort": "desc"},
            },
        },
        {
            "id": 13,
            "type": "stat",
            "title": "Collector",
            "description": "Per account, not per provider: one stale account used to colour the whole provider red without saying which. While this is stale every quota above is old, and their calm means nothing.",
            "datasource": DS,
            "gridPos": {"h": 4, "w": 5, "x": 19, "y": 16},
            "targets": [target("A", f'max by (provider, account_id) (loomwatch_agent_healthy{{account_id=~"$account"}})',
                               legend="{{provider}} / acct {{account_id}}")],
            "fieldConfig": {
                "defaults": {
                    "mappings": [{"type": "value", "options": {
                        "0": {"text": "stale", "color": "red", "index": 0},
                        "1": {"text": "fresh", "color": "green", "index": 1}}}],
                    "thresholds": {"mode": "absolute", "steps": [{"color": "red", "value": None}, {"color": "green", "value": 1}]},
                },
                "overrides": [],
            },
            "options": {
                "colorMode": "value", "graphMode": "none", "textMode": "value_and_name",
                "justifyMode": "auto",
                # One bit of information does not need display type. Fixed sizes
                # stop it growing to fill whatever space is left.
                "text": {"titleSize": 12, "valueSize": 22},
                "orientation": "horizontal",
                "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
            },
        },
    ]


def variables():
    def query(name, label, q, description):
        return {
            "name": name, "label": label, "type": "query", "datasource": DS,
            "query": q, "refresh": 1, "includeAll": True, "multi": True,
            "allValue": ".+", "sort": 1, "description": description,
            "current": {"text": ["All"], "value": ["$__all"]},
        }

    return [
        {"name": "datasource", "label": "Data source", "type": "datasource",
         "query": "prometheus", "current": {}},
        query("provider", "Provider", "label_values(loomwatch_agent_healthy, provider)",
              "Filters the table above and which accounts get a block below."),
        # The blocks repeat over this one.
        #
        # label_join builds the readable identity; the regex splits it again, so
        # the block is titled "zai / 13" while the queries inside get the bare
        # id. It has to be the classic query type: label_values resolves through
        # /api/v1/series, which takes a selector and rejects a function outright.
        #
        # The regex has no space after the comma. Grafana prints the series of a
        # query_result without one, and a regex written for the spaced form
        # matches nothing - which is not an error, it is a variable that comes
        # back empty and a dashboard that renders as one block called "All".
        dict(query("account", "Account",
                   {
                       "qryType": 5,
                       "query": 'query_result(label_join(loomwatch_agent_healthy{provider=~"$provider"}, "account", " / ", "provider", "account_id"))',
                       "refId": "PrometheusVariableQueryEditor-VariableQuery",
                   },
                   "One block per account. An account is what somebody pays for and adds one at a time; a provider is a category."),
             regex='/account="(?<text>[^"]+)",account_id="(?<value>[^"]+)"/'),
        query("quota_type", "Quota window", f'label_values(loomwatch_quota_utilization_percent{{provider=~"$provider"}}, quota_type)',
              "Rolling five-hour windows sit at zero most of the time; hide them here rather than dropping them from the data."),
        query("team", "Team", "label_values(loomwatch:account_team, team)",
              "Empty until account ownership is mapped in the chart: ownership is a recording rule, not something the collector exports."),
    ]


def main():
    row = account_row()
    dashboard = {
        "uid": "loomwatch-quotas",
        "title": "loomwatch - LLM quotas",
        "description": (
            "Remaining LLM subscription quota per provider, quota window and "
            "account. Utilisation is a share of each plan's own limit, so 100 "
            "is the quota itself."
        ),
        "tags": ["loomwatch", "llm", "quota"],
        "timezone": "browser",
        "schemaVersion": 39,
        "refresh": "1m",
        "time": {"from": "now-24h", "to": "now"},
        "templating": {"list": variables()},
        "panels": [triage_table(), row] + per_account_panels(),
    }
    out = "charts/loomwatch/dashboards/loomwatch.json"
    with open(out, "w") as fh:
        json.dump(dashboard, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    print(f"{out} written: {len(dashboard['panels'])} panels")


if __name__ == "__main__":
    main()

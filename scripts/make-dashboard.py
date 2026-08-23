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
    """Where this quota lands when its own window resets.

    Three things here were wrong before and are load-bearing now.

    Every term is aggregated to the same label set. Taking max of each term
    separately takes utilisation from one pod generation, the slope from
    another and the remaining time from a third, and adds up a forecast for a
    series that never existed. On a stand with five generations behind a single
    account that read 38% where the truth was 2%.

    The slope comes from a subquery over the aggregate, so restarts collapse
    before differentiating rather than after.

    The window is fixed at 24h to match burn.trendWindow in the alert. It used
    to be $__range, which made the column a function of the time picker: the
    same quota at the same moment read 2% on a six-hour view and 38% on a
    day's, with nothing on screen to say the number had moved.

    Clamped at 100 because utilisation is a share of the plan's own limit, and
    111% of a limit is not a quantity.
    """
    agg = f"max by (provider, quota_type, account_id) (loomwatch_quota_utilization_percent{{{sel}}})"
    return f"""
      clamp_max(
        {agg}
        + deriv(({agg})[24h:5m])
          * max by (provider, quota_type, account_id) (loomwatch_quota_reset_timestamp_seconds{{{sel}}} - time())
      , 100)
    """


def triage_table():
    return {
        "id": 1,
        "type": "table",
        "title": "Quotas, fullest first",
        "description": (
            "Every quota, ordered by how full it is. The title says fullest "
            "rather than soonest on purpose: a quota at 0% whose window resets "
            "in an hour is the safest thing on the board, so ordering by time "
            "would put it on top. Fullness is what needs an answer; the reset "
            "time beside it is what tells you how long you have.\n\n"
            "Utilisation is a share of each plan's own limit, so 100 is the "
            "quota itself - and for the same reason two rows at 100% are not "
            "comparable quantities and none of these numbers can be summed.\n\n"
            "At reset is a forecast from the last 24 hours, the same window the "
            "burn alert uses. It is blank where the provider publishes no reset "
            "time, because there is then no moment to forecast to."
        ),
        "datasource": DS,
        "gridPos": {"h": 15, "w": 24, "x": 0, "y": 0},
        "targets": [
            target("A", f"max by (provider, quota_type, account_id) (loomwatch_quota_utilization_percent{{{SEL}}})", instant=True),
            target("B", f"max by (provider, quota_type, account_id) (loomwatch_quota_reset_timestamp_seconds{{{SEL}}} - time())", instant=True),
            target("C", projection(SEL), instant=True),
            # Ownership, where the chart publishes it. No variable goes with
            # this: a filter that cannot filter, because the recording rule is
            # not installed, teaches the reader to distrust the filters beside
            # it that do work. The column simply appears when the data does.
            target("D", f"""
                max by (provider, quota_type, account_id, team) (
                  loomwatch_quota_utilization_percent{{{SEL}}}
                  * on (provider, account_id) group_left(team) loomwatch:account_team
                ) * 0
            """, instant=True),
            # The account's name, with its id as the fallback. The blocks below
            # are titled by name; a table that names the same account by number
            # makes the reader do the mapping in their head.
            target("E", """
                max by (provider, account_id, account_name) (
                  loomwatch_account_info
                  or label_replace(
                       loomwatch_agent_healthy unless on (provider, account_id) loomwatch_account_info,
                       "account_name", "$1", "account_id", "(.*)")
                )
            """, instant=True),
        ],
        "transformations": [
            {"id": "merge", "options": {}},
            {"id": "organize", "options": {
                "excludeByName": {"Time": True, "Value #D": True, "Value #E": True, "account_id": True},
                "renameByName": {
                    "provider": "Provider",
                    "account_name": "Account",
                    "quota_type": "Window",
                    "team": "Team",
                    "Value #A": "Utilisation",
                    "Value #B": "Resets in",
                    "Value #C": "At reset",
                },
                "indexByName": {
                    "provider": 0, "account_name": 1, "quota_type": 2, "team": 3,
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
                     {"id": "custom.cellOptions", "value": {"type": "gauge", "mode": "basic"}},
                     {"id": "thresholds", "value": {"mode": "absolute", "steps": STEPS}},
                 ]},
                {"matcher": {"id": "byName", "options": "Resets in"},
                 "properties": [{"id": "unit", "value": "s"}, {"id": "noValue", "value": "not published"}]},
                {"matcher": {"id": "byName", "options": "At reset"},
                 "properties": [
                     {"id": "unit", "value": "percent"}, {"id": "decimals", "value": 0},
                     {"id": "custom.cellOptions", "value": {"type": "color-text"}},
                     {"id": "noValue", "value": "-"},
                     {"id": "thresholds", "value": {"mode": "absolute", "steps": STEPS}},
                 ]},
                {"matcher": {"id": "byName", "options": "Provider"}, "properties": [{"id": "custom.width", "value": 120}]},
                {"matcher": {"id": "byName", "options": "Account"}, "properties": [{"id": "custom.width", "value": 150}]},
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
    psel = 'provider=~"$provider",account_id=~"$account",quota_type=~"$quota_type"' 
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
                "displayMode": "basic",
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
            "description": (
                "Two readings, because one of them cannot see the failure the "
                "other alerts on. agent_healthy is the collector's own verdict "
                "about this account. Age of last poll is what "
                "LoomwatchCollectorNotPolling is built on, and that rule exists "
                "precisely for the case where the collector stalls while its "
                "own health flag stays at 1 - so a panel showing only the flag "
                "reports fresh at the exact moment the alert fires.\n\n"
                "While this is stale every quota beside it is old, and their "
                "calm stops meaning anything."
            ),
            "datasource": DS,
            "gridPos": {"h": 9, "w": 5, "x": 19, "y": 16},
            "targets": [
                target("A", f'max by (account_id) (loomwatch_agent_healthy{{provider=~"$provider",account_id=~"$account"}})',
                       legend="health"),
                target("B", f'max by (account_id) (loomwatch_agent_last_cycle_age_seconds{{provider=~"$provider",account_id=~"$account"}})',
                       legend="last poll"),
            ],
            "fieldConfig": {
                "defaults": {"thresholds": {"mode": "absolute", "steps": [{"color": "green", "value": None}]}},
                "overrides": [
                    {"matcher": {"id": "byFrameRefID", "options": "A"},
                     "properties": [
                         {"id": "mappings", "value": [{"type": "value", "options": {
                             "0": {"text": "stale", "color": "red", "index": 0},
                             "1": {"text": "fresh", "color": "green", "index": 1}}}]},
                         {"id": "thresholds", "value": {"mode": "absolute", "steps": [
                             {"color": "red", "value": None}, {"color": "green", "value": 1}]}},
                     ]},
                    {"matcher": {"id": "byFrameRefID", "options": "B"},
                     "properties": [
                         {"id": "unit", "value": "s"},
                         # The rule's own threshold shape: five poll intervals.
                         {"id": "thresholds", "value": {"mode": "absolute", "steps": [
                             {"color": "green", "value": None}, {"color": "red", "value": 300}]}},
                     ]},
                ],
            },
            "options": {
                "colorMode": "value", "graphMode": "none", "textMode": "value_and_name",
                "justifyMode": "auto",
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
        # the block is titled "zai / spare-max" while the queries inside get the
        # bare id. It has to be the classic query type: label_values resolves
        # through /api/v1/series, which takes a selector and rejects a function
        # outright.
        #
        # The name comes from loomwatch_account_info, which exists only for the
        # providers that keep account rows. The second branch carries everyone
        # else by id: a block titled "gemini / default" is worse than one titled
        # "gemini / spare-max", and both are far better than no block at all.
        #
        # The regex has no space after the comma. Grafana prints the series of a
        # query_result without one, and a regex written for the spaced form
        # matches nothing - which is not an error, it is a variable that comes
        # back empty and a dashboard that renders as one block called "All".
        dict(query("account", "Account",
                   {
                       "qryType": 5,
                       "query": (
                           "query_result("
                           # Named accounts, where the collector keeps rows for them.
                           'label_join(loomwatch_agent_healthy{provider=~"$provider"}'
                           " * on(provider, account_id) group_left(account_name) loomwatch_account_info,"
                           ' "account", " / ", "provider", "account_name")'
                           " or "
                           # Everyone else, by id. Without this branch the seven
                           # providers that keep no account rows lose their blocks
                           # entirely, which is a worse trade than an ugly label.
                           'label_join(loomwatch_agent_healthy{provider=~"$provider"}'
                           " unless on(provider, account_id) loomwatch_account_info,"
                           ' "account", " / ", "provider", "account_id")'
                           ")"
                       ),
                       "refId": "PrometheusVariableQueryEditor-VariableQuery",
                   },
                   "One block per account. An account is what somebody pays for and adds one at a time; a provider is a category."),
             regex='/account="(?<text>[^"]+)",account_id="(?<value>[^"]+)"/'),
        query("quota_type", "Quota window", f'label_values(loomwatch_quota_utilization_percent{{provider=~"$provider"}}, quota_type)',
              "Rolling five-hour windows sit at zero most of the time; hide them here rather than dropping them from the data."),
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

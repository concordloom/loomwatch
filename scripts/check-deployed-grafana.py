#!/usr/bin/env python3
"""Read-only browser check of the quota dashboard, as Grafana actually draws it.

Why this exists, precisely.

Three defects shipped to a deployed board on 24 August and were found by a human
looking at it, after every automated contour had passed. Every reset time
rendered as 21 January 1970, because Grafana's date units read milliseconds and
the metric is in seconds. Two rows had an empty Account, because the name
arrived as its own frame and the merge transformation attaches such a frame to
one row per key. And the placeholder for "this quota is not on track to breach"
- the safe state, and most of the rows - was painted in alarm red, because
Grafana renders an absent value in the base threshold colour and the base was
red.

None of the three is visible to anything that inspects queries. promtool parses
the PromQL and it was valid. The queries were run against the live Prometheus
and returned correct numbers. helm unittest read the rendered JSON and the field
config was exactly what the generator intended. All three live in the last step,
where a correct number meets a formatter, and the only instrument that sees that
step is a browser.

So this check asserts about rendering and nothing else. It deliberately does not
re-check the queries: that is covered, and duplicating it here would make the
check look thorough while still missing the class it exists for.

Nothing about any particular deployment belongs in this file. The target and the
credentials come from the environment:

    LOOMWATCH_GRAFANA_URL       base URL of Grafana, e.g. http://127.0.0.1:13000
    LOOMWATCH_GRAFANA_USER      login (default: admin)
    LOOMWATCH_GRAFANA_PASS      password
    LOOMWATCH_GRAFANA_UID       dashboard uid (default: the uid in the chart's
                                own dashboard, so a fork that renames it needs
                                no change here)
    LOOMWATCH_UI_ARTIFACTS      screenshot directory (default: a temporary one)

The panels it expects are read from the dashboard the chart ships, not listed
here, so this stays correct for a deployment that carries a different set.

Requires the playwright package - the same one tests/e2e/requirements.txt
installs.
"""
import datetime
import json
import os
import pathlib
import re
import sys
import tempfile

from playwright.sync_api import sync_playwright

ROOT = pathlib.Path(__file__).resolve().parent.parent
DASHBOARD = ROOT / "charts" / "loomwatch" / "dashboards" / "loomwatch.json"

# A date drawn by this board is a quota reset: the recent past or the near
# future. The window is deliberately loose - the point is not to pin the value,
# it is to catch a timestamp that has been read in the wrong unit, and those
# miss by decades rather than by days.
SANE_YEARS_BACK = 2
SANE_YEARS_FORWARD = 5

# Matches the way Grafana's dateTime units render: a four-digit year somewhere
# in a cell that also carries a time. Cells that are plain durations ("5 hours")
# or plain text are ignored, which is why the check reports how many it found.
DATE_WITH_YEAR = re.compile(r"\b(\d{4})\b")
LOOKS_LIKE_TIME = re.compile(r"\b\d{1,2}:\d{2}\b")


def require(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        sys.exit(f"{name} is not set; this check needs a Grafana to talk to")
    return value


def is_alarm_red(css_colour: str) -> bool:
    """Whether a computed CSS colour reads as an alarm.

    A range rather than one palette entry: Grafana's red differs between
    versions and themes, and the claim being tested is "this does not shout at
    the operator", not "this is not exactly #F2495C".
    """
    match = re.match(r"rgba?\((\d+),\s*(\d+),\s*(\d+)", css_colour or "")
    if not match:
        return False
    r, g, b = (int(match.group(i)) for i in (1, 2, 3))
    return r > 170 and g < 110 and b < 110


def main() -> int:
    url = require("LOOMWATCH_GRAFANA_URL").rstrip("/")
    user = os.environ.get("LOOMWATCH_GRAFANA_USER", "admin")
    password = require("LOOMWATCH_GRAFANA_PASS")

    shipped = json.loads(DASHBOARD.read_text())
    uid = os.environ.get("LOOMWATCH_GRAFANA_UID", "").strip() or shipped["uid"]
    # Titles of the panels that are visible without expanding anything. A
    # collapsed row's contents are not drawn, and asserting on them would fail
    # for a reason that has nothing to do with rendering.
    expected_titles = [
        p["title"] for p in shipped["panels"]
        if p.get("type") != "row" and p.get("title")
    ]

    artifacts = pathlib.Path(
        os.environ.get("LOOMWATCH_UI_ARTIFACTS") or tempfile.mkdtemp(prefix="loomwatch-grafana-")
    )
    artifacts.mkdir(parents=True, exist_ok=True)

    failures: list[str] = []

    def check(condition: bool, message: str) -> None:
        if not condition:
            failures.append(message)

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        page = browser.new_page(viewport={"width": 1800, "height": 1200})

        page.goto(f"{url}/login", wait_until="domcontentloaded", timeout=30_000)
        page.fill('input[name="user"]', user)
        page.fill('input[name="password"]', password)
        page.click('button[type="submit"]')
        page.wait_for_timeout(3_000)

        page.goto(f"{url}/d/{uid}?kiosk", wait_until="networkidle", timeout=60_000)
        # Panels resolve their queries after the page settles; a table that has
        # not painted yet is indistinguishable from one that painted empty.
        page.wait_for_timeout(6_000)
        page.screenshot(path=str(artifacts / "dashboard.png"), full_page=True)

        body = page.inner_text("body")

        # The dashboard is there at all. Without this every assertion below
        # passes vacuously on a 404 page, which is the failure mode the oracle
        # rules exist to forbid.
        check(
            "Dashboard not found" not in body and len(body) > 200,
            f"{url}/d/{uid} did not render a dashboard",
        )

        for title in expected_titles:
            check(title in body, f"panel {title!r} is missing from the rendered board")

        # Grafana reports a panel that could not run inside the panel itself.
        for marker in ("Error updating options", "query error", "Templating"):
            check(marker not in body, f"a panel shows {marker!r}")

        # --- the three defects this file exists for ---

        cells = page.query_selector_all('[role="gridcell"], [role="cell"]')
        check(len(cells) > 0, "the table drew no cells; nothing below this was actually checked")

        texts = [(c, (c.inner_text() or "").strip()) for c in cells]

        # 1. Dates, in the unit the formatter actually read.
        now = datetime.date.today().year
        dated = [t for _, t in texts if LOOKS_LIKE_TIME.search(t) and DATE_WITH_YEAR.search(t)]
        check(
            len(dated) > 0,
            "no cell rendered a date; the date assertion below proved nothing",
        )
        for text in dated:
            year = int(DATE_WITH_YEAR.search(text).group(1))
            check(
                now - SANE_YEARS_BACK <= year <= now + SANE_YEARS_FORWARD,
                f"a cell renders {text!r}: year {year} is not a plausible quota reset. "
                "A Unix timestamp in seconds fed to a millisecond formatter lands in 1970.",
            )

        # 2. Blank cells in a row that has data. A row Grafana drew at all has a
        #    provider; an empty neighbour means a join or a transformation
        #    dropped the value for that row only, which is invisible in the
        #    query results because the query is not where it was lost.
        rows = page.query_selector_all('[role="row"]')
        populated = 0
        for row in rows:
            row_cells = [(c.inner_text() or "").strip() for c in row.query_selector_all('[role="gridcell"], [role="cell"]')]
            if not row_cells or not row_cells[0]:
                continue
            populated += 1
            blanks = [i for i, value in enumerate(row_cells) if value == ""]
            check(
                not blanks,
                f"row {row_cells!r} has empty cells at {blanks}: a value was lost for this row only",
            )
        check(populated > 0, "no populated row was examined; the blank-cell assertion proved nothing")

        # 3. A safe state must not be painted as an alarm. The placeholder text
        #    comes from the shipped dashboard rather than being written here, so
        #    renaming it in the generator cannot silently disable this.
        placeholders = {
            prop["value"]
            for panel in shipped["panels"]
            for override in panel.get("fieldConfig", {}).get("overrides", [])
            for prop in override.get("properties", [])
            if prop.get("id") == "noValue" and isinstance(prop.get("value"), str)
        }
        checked_placeholders = 0
        for cell, text in texts:
            if text not in placeholders:
                continue
            checked_placeholders += 1
            colour = cell.evaluate("el => getComputedStyle(el).color")
            check(
                not is_alarm_red(colour),
                f"the placeholder {text!r} is drawn in {colour}, which reads as an alarm. "
                "Grafana paints an absent value with the base threshold colour.",
            )
        if placeholders:
            check(
                checked_placeholders > 0,
                f"none of the placeholders {sorted(placeholders)} appeared, so the "
                "colour assertion proved nothing about them",
            )

        browser.close()

    if failures:
        print(f"grafana dashboard check FAILED ({len(failures)}):", file=sys.stderr)
        for failure in failures:
            print(f"  - {failure}", file=sys.stderr)
        print(f"screenshots: {artifacts}", file=sys.stderr)
        return 1

    print(
        f"deployed dashboard ok: {len(expected_titles)} panels, {populated} rows, "
        f"{len(dated)} dates in range, {checked_placeholders} placeholders not in alarm colour"
    )
    print(f"screenshots: {artifacts}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

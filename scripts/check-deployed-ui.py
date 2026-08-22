#!/usr/bin/env python3
"""Read-only browser check of a deployed loomWatch instance.

The browser suite in tests/e2e builds its own binary and drives it on
loopback: it proves the interface on a workstation and says nothing about the
instance people actually use. This drives a real browser against a running
deployment instead, which is what Stage 2 is for.

Nothing about any particular deployment belongs in this file. The target and
the credentials come from the environment:

    LOOMWATCH_STAND_URL       base URL of the deployment, e.g. https://example
    LOOMWATCH_ADMIN_USER      dashboard user (default: admin)
    LOOMWATCH_ADMIN_PASS      dashboard password
    LOOMWATCH_EXPECT_VERSION  version it must report (default: the VERSION file)
    LOOMWATCH_UI_ARTIFACTS    screenshot directory (default: a temporary one)

Screenshots go to a temporary directory by default deliberately: they carry
the address and live quota figures, so they do not belong next to the code.

The wrong-password check runs before the real login, not after. The server
blocks an address after five failed attempts and clears the counter on a
successful login, so this order never leaves a deployment holding a spent
attempt budget.

Requires the playwright package - the same one tests/e2e/requirements.txt
installs.
"""
import os
import pathlib
import sys
import tempfile

from playwright.sync_api import sync_playwright

ROOT = pathlib.Path(__file__).resolve().parent.parent


def require(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        sys.exit(f"{name} is not set; this check needs a deployment to talk to")
    return value


def main() -> int:
    url = require("LOOMWATCH_STAND_URL").rstrip("/")
    user = os.environ.get("LOOMWATCH_ADMIN_USER", "admin")
    password = require("LOOMWATCH_ADMIN_PASS")
    expected = os.environ.get("LOOMWATCH_EXPECT_VERSION", "").strip()
    if not expected:
        expected = (ROOT / "VERSION").read_text().strip()
    artifacts = pathlib.Path(
        os.environ.get("LOOMWATCH_UI_ARTIFACTS") or tempfile.mkdtemp(prefix="loomwatch-ui-")
    )
    artifacts.mkdir(parents=True, exist_ok=True)

    failures: list[str] = []
    bad_responses: list[str] = []
    failed_requests: list[str] = []

    def check(condition: bool, message: str) -> None:
        if not condition:
            failures.append(message)

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        page = browser.new_page()

        # A wrong password must not get in. Without this the whole pass could
        # be green against an instance that authenticates nobody.
        page.goto(f"{url}/login", wait_until="networkidle", timeout=30_000)
        page.fill("#username", user)
        page.fill("#password", f"{password}-definitely-wrong")
        page.click("button.login-button")
        page.wait_for_timeout(2_000)
        check(
            "/login" in page.url,
            f"a wrong password reached {page.url}; the dashboard is not actually protected",
        )

        # Only now the real login, which also clears the failed attempt above.
        #
        # Responses are collected from the submit onwards and only while the
        # browser is off the login page: an unauthenticated login page asks for
        # /api/settings and is answered 401 by design, and counting that would
        # make the check red on a healthy deployment.
        watching = False

        def on_response(response) -> None:
            if watching and response.status >= 400 and "/login" not in page.url:
                bad_responses.append(f"{response.status} {response.url.replace(url, '')}")

        page.on("response", on_response)
        page.on(
            "requestfailed",
            lambda r: failed_requests.append(f"{r.method} {r.url.replace(url, '')} {r.failure}")
            if watching
            else None,
        )
        page.goto(f"{url}/login", wait_until="networkidle", timeout=30_000)
        page.fill("#username", user)
        page.fill("#password", password)
        page.click("button.login-button")
        watching = True
        try:
            page.wait_for_url(f"{url}/", timeout=15_000)
            page.wait_for_selector(".app-header", timeout=15_000)
        except Exception as exc:  # noqa: BLE001 - the message is the report
            browser.close()
            print(f"login did not reach the dashboard: {exc}")
            return 1
        page.wait_for_timeout(4_000)

        footer = page.query_selector(".footer-brand")
        footer_text = footer.inner_text().strip() if footer else ""
        check(
            expected in footer_text,
            f"the dashboard reports {footer_text!r}, not version {expected}",
        )

        cards = page.query_selector_all("article.quota-card")
        percentages = [
            c.query_selector(".usage-percent").inner_text().strip()
            for c in cards
            if c.query_selector(".usage-percent")
        ]
        check(bool(percentages), "no quota card rendered a percentage")

        page.evaluate("document.getElementById('cycles-section')?.scrollIntoView()")
        page.wait_for_timeout(3_000)
        rows = page.locator("#cycles-table tbody tr").count()
        check(rows > 0, "the cycles table is empty; the deployment collected nothing")
        page.screenshot(path=str(artifacts / "dashboard.png"), full_page=True)

        page.goto(f"{url}/settings", wait_until="networkidle", timeout=30_000)
        try:
            page.wait_for_selector(".settings-page", timeout=15_000)
        except Exception:  # noqa: BLE001
            failures.append("the settings page did not render")
        page.screenshot(path=str(artifacts / "settings.png"), full_page=True)

        browser.close()

    check(not bad_responses, f"failed responses after login: {bad_responses}")
    check(not failed_requests, f"requests failed after login: {failed_requests}")

    if failures:
        for failure in failures:
            print(f"FAIL {failure}")
        print(f"screenshots: {artifacts}")
        return 1

    print(
        f"deployed UI ok: version {expected}, {len(percentages)} quota cards, "
        f"{rows} cycle rows, settings rendered, no failed responses"
    )
    print(f"screenshots: {artifacts}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

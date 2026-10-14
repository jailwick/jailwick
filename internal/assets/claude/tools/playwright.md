
## Browser testing and screenshots (Playwright)
- The project's browsers are preinstalled at `$PLAYWRIGHT_BROWSERS_PATH`, matching the global
  `playwright` CLI and the `playwright` MCP server.
- One-off capture: `playwright screenshot --full-page <url> /tmp/shot.png`, then view the PNG
  with the Read tool. Use the global `playwright` here, not `npx playwright`.
- Interactive: the `playwright` MCP server is configured (navigate, click, fill, screenshot).
- Project tests: `npx playwright test` uses the project's own version. If a browser is missing,
  `npx playwright install <browser>` once (downloads through the proxy).
- HTML report: `npx playwright show-report --host 0.0.0.0 --port $PORT`.

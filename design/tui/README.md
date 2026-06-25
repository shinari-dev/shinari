# TUI design mockups

Bootstrap `.tui` files for the **scenario workbench** redesign, authored for
[TUI Studio](https://github.com/jalonsogo/tui-studio) (the Figma-like TUI designer).
Design of record: [`../../spec-tui-scenario-workbench.md`](../../spec-tui-scenario-workbench.md).

## The loop

1. Start TUI Studio (`npm run dev` in the tui-studio repo, opens `localhost:5173`).
2. Open a file here with **Cmd/Ctrl+O**.
3. Art-direct on the canvas (layout, spacing, colors, components).
4. Save back over the same file with **Cmd/Ctrl+S**.
5. The refined `.tui` becomes the reference the Go implementation is built against.

## Files

| File | Screen |
|---|---|
| `splash.tui` | hero logo: the unbroken line deflecting around the ember fault dot, wordmark + tagline |
| `scenarios.tui` | scenario list + detail panel, status bar, tab strip, key bar |
| `run-with-logs.tui` | live run panel with the steps / event-log split |
| `history.tui` | findings-ledger trend table |
| `new-scenario.tui` | the scaffold modal |

## Palette (sourced from `docs/assets/css/main.css` + logo)

| Role | Hex |
|---|---|
| Canvas | `#0c0e12` |
| Panel fill | `#13151c` |
| Focused border / accent / FINDING | `#ff4f2b` |
| Unfocused border | `#2c2f37` |
| Text (= logo line) | `#eceae6` |
| Soft / dim text | `#b4b7bd` / `#82868e` |
| PASSED | `#3ecf8e` |
| FAILED | `#ff5c57` |
| Warning | `#ffb454` |
| Info (log meta) | `#62b3f0` |

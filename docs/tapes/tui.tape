# SPDX-License-Identifier: Apache-2.0
# © The Shinari Authors
#
# Shinari TUI demo — recorded with charmbracelet/vhs (https://github.com/charmbracelet/vhs).
#
# Regenerate with `make demo` from the repo root (builds ./shinari first, then
# renders this tape). Requires: vhs, ttyd, ffmpeg, and docker (the scenario below
# brings up a Redis container). Emits docs/assets/media/tui.{mp4,gif}.
#
# It drives the interactive control center against examples/faults, featuring
# cache-survives-cold-restart — the homepage's hero story: docker.stop the Redis
# cache mid-flight, bring it back cold, and prove reads come back as a miss (not a
# connection error) and repopulate. Native docker.* verbs, matching the static
# showcase and the verb grid on the page. The narrative: browse → read the plan →
# run → watch steady → fault (cache down) → recovery → verify → PASSED stream live.
#
# cache-survives-cold-restart is the first row (suite "cache" sorts first). The run
# is real — docker stop/start take a couple of seconds — so the Sleeps around it
# both pace the readout and cover the live stream. Retune against a render if the
# faults scenario set changes.

Output docs/assets/media/tui.mp4
Output docs/assets/media/tui.gif

Set Shell   "bash"
Set FontSize 15
Set Width    1220
Set Height   760
Set Padding  28
Set TypingSpeed 55ms

# Match the docs' dark ember palette (see docs/assets/css/main.css :root).
Set Theme { "name": "shinari", "background": "#0c0e12", "foreground": "#e8e6e1", "cursor": "#ff4f2b", "selection": "#1c1f27", "black": "#0a0b0e", "red": "#ff4f2b", "green": "#3ecf8e", "yellow": "#ffb454", "blue": "#5b8bd0", "magenta": "#ff7a58", "cyan": "#4fd0c7", "white": "#eceae6", "brightBlack": "#82868e", "brightRed": "#ff7a58", "brightGreen": "#3ecf8e", "brightYellow": "#ffb454", "brightBlue": "#7aa2e0", "brightMagenta": "#ff9a7a", "brightCyan": "#6fe0d7", "brightWhite": "#ffffff" }

# --- launch -------------------------------------------------------------------
Type "./shinari -p examples/faults tui"
Enter
Sleep 1300ms          # splash logo

Space                 # dismiss splash → Scenarios screen
Sleep 1700ms          # let the table + overview settle

# --- browse (cache-survives-cold-restart is the first row, already selected) ---
Down
Sleep 700ms           # peek the next scenario — the list is live
Up
Sleep 1000ms          # back on cache-survives-cold-restart

# --- read the plan ------------------------------------------------------------
Right
Sleep 1900ms          # Overview → Explain: the rendered run plan
Left
Sleep 700ms

# --- run it live --------------------------------------------------------------
Type "r"              # run the selected scenario
Sleep 4500ms          # steady → warm → docker.stop (cache down) → cold restart → verify
Sleep 3000ms          # verdict banner: PASSED — held long enough to read

Escape                # back to the scenario list — the resting frame, so the loop
Sleep 2200ms          # restarts cleanly from the table (no quit-to-black flash)

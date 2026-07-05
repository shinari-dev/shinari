// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/shinari-dev/shinari/core/engine"
)

const (
	docsURL   = "https://shinari.dev/"
	githubURL = "https://github.com/shinari-dev/shinari"
)

// HTML writes report.html: a self-contained rendering of the run — inline CSS
// and one small inline script, no external assets — so the single file can be
// attached to a Slack thread or CI artifact and opens offline.
func HTML(w io.Writer, res engine.RunResult) error {
	return htmlTmpl.Execute(w, newHTMLRun(res))
}

// htmlRun is the template's view of a RunResult: durations and counts are
// precomputed so the template stays free of logic.
type htmlRun struct {
	Verdict      engine.ScenarioVerdict
	Started      string
	Duration     string
	Total        int
	Passed       int
	Failed       int
	Errored      int
	Inconclusive int
	Findings     int
	Scenarios    []htmlScenario
	MarkdownJS   template.JS // per-scenario LLM markdown, indexed by htmlScenario.Index
	DocsURL      string
	GithubURL    string
}

type htmlScenario struct {
	Index       int // position in MarkdownJS, wired to the copy button
	Name        string
	Suite       string
	Description string
	Reason      string
	Verdict     engine.ScenarioVerdict
	Duration    string
	Injected    []string
	Held        []string
	Findings    []engine.FindingRecord
	Steps       []htmlStep
	Open        bool // expand the scenario when it needs attention
}

type htmlStep struct {
	Section  string
	Label    string
	Verdict  engine.CheckVerdict
	Duration string
	Detail   string
}

func newHTMLRun(res engine.RunResult) htmlRun {
	run := htmlRun{
		Verdict:   res.Verdict(),
		Started:   res.Start.UTC().Format("2006-01-02 15:04:05 UTC"),
		Duration:  fmtDur(res.End.Sub(res.Start)),
		Total:     len(res.Scenarios),
		DocsURL:   docsURL,
		GithubURL: githubURL,
	}
	markdown := make([]string, 0, len(res.Scenarios))
	for i, sc := range res.Scenarios {
		switch sc.Verdict {
		case engine.ScenarioPassed:
			run.Passed++
		case engine.ScenarioFailed:
			run.Failed++
		case engine.ScenarioErrored:
			run.Errored++
		case engine.ScenarioInconclusive:
			run.Inconclusive++
		}
		// count active gaps only, matching the console summary and SARIF — a
		// now-passing finding is a promotion prompt, not an open gap
		for _, f := range sc.Findings {
			if !f.NowPasses {
				run.Findings++
			}
		}

		hs := htmlScenario{
			Index: i,
			Name:  sc.Name, Suite: sc.Suite, Description: sc.Description,
			Reason: sc.Reason, Verdict: sc.Verdict,
			Duration: fmtDur(sc.End.Sub(sc.Start)),
			Injected: sc.Injected, Held: sc.Held, Findings: sc.Findings,
			Open: sc.Verdict != engine.ScenarioPassed,
		}
		for _, st := range sc.Steps {
			detail := st.Err
			if detail == "" {
				detail = st.SkipReason
			}
			if st.Verdict == engine.CheckFinding {
				detail = st.Finding
			}
			hs.Steps = append(hs.Steps, htmlStep{
				Section: st.Section, Label: st.Label(), Verdict: st.Verdict,
				Duration: fmtDur(st.End.Sub(st.Start)), Detail: detail,
			})
		}
		run.Scenarios = append(run.Scenarios, hs)
		markdown = append(markdown, scenarioMarkdown(hs))
	}
	// json.Marshal HTML-escapes <, >, & to \uXXXX, so the array is safe to
	// inline in a <script> without a </script> breakout.
	buf, _ := json.Marshal(markdown)
	run.MarkdownJS = template.JS(buf)
	return run
}

// scenarioMarkdown renders one scenario as a compact markdown block sized for
// pasting into an LLM: header, ledger, then the step table.
func scenarioMarkdown(sc htmlScenario) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n", sc.Name)
	meta := fmt.Sprintf("verdict: %s · duration: %s", sc.Verdict, sc.Duration)
	if sc.Suite != "" {
		meta = "suite: " + sc.Suite + " · " + meta
	}
	fmt.Fprintf(&b, "_%s_\n", meta)
	if sc.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", sc.Description)
	}
	if sc.Reason != "" {
		fmt.Fprintf(&b, "\n**Reason:** %s\n", sc.Reason)
	}
	if len(sc.Injected) > 0 {
		b.WriteString("\n**Injected**\n")
		for _, v := range sc.Injected {
			fmt.Fprintf(&b, "- `%s`\n", v)
		}
	}
	if len(sc.Held) > 0 {
		b.WriteString("\n**Held**\n")
		for _, v := range sc.Held {
			fmt.Fprintf(&b, "- %s\n", v)
		}
	}
	if len(sc.Findings) > 0 {
		b.WriteString("\n**Findings**\n")
		for _, f := range sc.Findings {
			if f.NowPasses {
				fmt.Fprintf(&b, "- %s (now passes: promote to a hard assertion)\n", f.Narrative)
			} else {
				fmt.Fprintf(&b, "- %s (observed: %s)\n", f.Narrative, f.Detail)
			}
		}
	}
	b.WriteString("\n**Steps**\n\n")
	b.WriteString("| section | check | verdict | duration | detail |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, st := range sc.Steps {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			mdCell(st.Section), mdCell(st.Label), st.Verdict, st.Duration, mdCell(st.Detail))
	}
	return b.String()
}

// mdCell keeps a value on one table row: pipes are escaped, newlines flattened.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.Join(strings.Fields(s), " ")
}

func fmtDur(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return "<1ms"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

var htmlTmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"plural": plural, // console.go's "s" suffix, so tiles and summaries read naturally
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Shinari report — {{.Verdict}}</title>
<style>
  :root {
    --bg: #0a0b0e; --surface: #14161c; --border: #23262f;
    --text: #e8e6e3; --muted: #9aa0ae; --ember: #ff4f2b;
    --pass: #3dd68c; --fail: #ff5c5c; --err: #ff8563;
    --finding: #f0b429; --skip: #9aa0ae;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; background: var(--bg); color: var(--text);
    font: 15px/1.55 system-ui, -apple-system, "Segoe UI", sans-serif;
  }
  main { max-width: 960px; margin: 0 auto; padding: 0 24px 48px; }
  code, td.mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }

  .top {
    display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap;
    max-width: 960px; margin: 0 auto; padding: 32px 24px 8px;
  }
  .brand { font-size: 22px; font-weight: 700; letter-spacing: .02em; }
  .brand em { color: var(--ember); font-style: normal; }
  .brand span { color: var(--muted); font-weight: 400; }
  .top .when { margin-left: auto; color: var(--muted); font-size: 13px; }

  .badge {
    display: inline-block; padding: 2px 10px; border-radius: 999px;
    font-size: 12px; font-weight: 700; letter-spacing: .06em;
    border: 1px solid;
  }
  .v-PASSED { color: var(--pass); border-color: color-mix(in srgb, var(--pass) 40%, transparent); background: color-mix(in srgb, var(--pass) 10%, transparent); }
  .v-FAILED { color: var(--fail); border-color: color-mix(in srgb, var(--fail) 40%, transparent); background: color-mix(in srgb, var(--fail) 10%, transparent); }
  .v-ERRORED { color: var(--err); border-color: color-mix(in srgb, var(--err) 40%, transparent); background: color-mix(in srgb, var(--err) 10%, transparent); }
  .v-INCONCLUSIVE { color: var(--skip); border-color: color-mix(in srgb, var(--skip) 40%, transparent); background: color-mix(in srgb, var(--skip) 10%, transparent); }

  .stats {
    display: flex; gap: 12px; flex-wrap: wrap;
    max-width: 960px; margin: 0 auto; padding: 16px 24px 24px;
  }
  .stat {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 10px; padding: 10px 16px; min-width: 96px;
  }
  .stat b { display: block; font-size: 20px; }
  .stat span { color: var(--muted); font-size: 12px; }
  .stat.pass b { color: var(--pass); } .stat.fail b { color: var(--fail); }
  .stat.err b { color: var(--err); } .stat.inc b { color: var(--skip); }
  .stat.finding b { color: var(--finding); }

  .scenario {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 12px; margin: 0 0 16px;
  }
  .scenario > summary {
    display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap;
    list-style: none; cursor: pointer; padding: 18px 20px;
  }
  .scenario > summary::-webkit-details-marker { display: none; }
  .scenario > summary::before {
    content: "▸"; color: var(--muted); font-size: 12px;
    align-self: center; transition: transform .12s ease;
  }
  .scenario[open] > summary::before { transform: rotate(90deg); }
  .scenario h2 { display: inline; margin: 0; font-size: 17px; }
  .suite { color: var(--muted); font-size: 13px; }
  .scenario > summary .dur { margin-left: auto; color: var(--muted); font-size: 13px; }
  .scenario .count { color: var(--muted); font-size: 13px; }
  .scenario .body { padding: 0 20px 18px; }
  .copy {
    font: inherit; font-size: 12px; cursor: pointer;
    color: var(--muted); background: transparent;
    border: 1px solid var(--border); border-radius: 6px; padding: 2px 10px;
  }
  .copy:hover { color: var(--text); border-color: var(--muted); }
  .copy.ok { color: var(--pass); border-color: color-mix(in srgb, var(--pass) 40%, transparent); }
  .desc { margin: 8px 0 0; color: var(--muted); }
  .reason { margin: 8px 0 0; color: var(--fail); }

  .ledger { display: flex; gap: 24px; flex-wrap: wrap; margin-top: 12px; }
  .ledger section { min-width: 200px; flex: 1; }
  .ledger h3 {
    margin: 0 0 4px; font-size: 11px; font-weight: 700;
    letter-spacing: .1em; text-transform: uppercase; color: var(--muted);
  }
  .ledger ul { margin: 0; padding-left: 18px; }
  .ledger li { margin: 2px 0; }
  .gapped li { color: var(--finding); }
  .gapped li .obs { color: var(--muted); font-size: 13px; }
  .gapped li.now-passes { color: var(--pass); }
  .gapped li.now-passes s { color: var(--muted); }

  .steps-head {
    display: flex; align-items: center; gap: 12px;
    margin-top: 14px; color: var(--muted); font-size: 13px;
  }
  .steps { overflow-x: auto; }
  table { border-collapse: collapse; width: 100%; margin-top: 8px; font-size: 13px; }
  th { text-align: left; color: var(--muted); font-weight: 600; }
  th, td { padding: 5px 12px 5px 0; border-bottom: 1px solid var(--border); vertical-align: top; }
  tr:last-child td { border-bottom: 0; }
  td.sec { color: var(--muted); white-space: nowrap; }
  td.dur { color: var(--muted); white-space: nowrap; text-align: right; }
  .c-PASS { color: var(--pass); } .c-FAIL { color: var(--fail); }
  .c-SKIP { color: var(--skip); } .c-FINDING { color: var(--finding); }

  footer {
    max-width: 960px; margin: 0 auto; padding: 0 24px 32px;
    color: var(--muted); font-size: 12px;
  }
  footer a { color: var(--muted); text-decoration: none; }
  footer a:hover { color: var(--ember); }
</style>
</head>
<body>
<div class="top">
  <div class="brand"><em>shinari</em> <span>run report</span></div>
  <span class="badge v-{{.Verdict}}">{{.Verdict}}</span>
  <span class="when">{{.Started}} · {{.Duration}}</span>
</div>
<div class="stats">
  <div class="stat"><b>{{.Total}}</b><span>scenario{{plural .Total}}</span></div>
  <div class="stat pass"><b>{{.Passed}}</b><span>passed</span></div>
  {{if .Failed}}<div class="stat fail"><b>{{.Failed}}</b><span>failed</span></div>{{end}}
  {{if .Errored}}<div class="stat err"><b>{{.Errored}}</b><span>errored</span></div>{{end}}
  {{if .Inconclusive}}<div class="stat inc"><b>{{.Inconclusive}}</b><span>inconclusive</span></div>{{end}}
  {{if .Findings}}<div class="stat finding"><b>{{.Findings}}</b><span>finding{{plural .Findings}}</span></div>{{end}}
</div>
<main>
{{range .Scenarios}}
  <details class="scenario"{{if .Open}} open{{end}}>
    <summary>
      <h2>{{.Name}}</h2>
      {{if .Suite}}<span class="suite">{{.Suite}}</span>{{end}}
      <span class="badge v-{{.Verdict}}">{{.Verdict}}</span>
      <span class="dur">{{.Duration}}</span>
      <button type="button" class="copy" data-i="{{.Index}}" onclick="shinariCopy(event, this)">copy for LLM</button>
    </summary>
    <div class="body">
      {{if .Description}}<p class="desc">{{.Description}}</p>{{end}}
      {{if .Reason}}<p class="reason">{{.Reason}}</p>{{end}}
      {{if or .Injected .Held .Findings}}
      <div class="ledger">
        {{if .Injected}}<section><h3>Injected</h3><ul>{{range .Injected}}<li><code>{{.}}</code></li>{{end}}</ul></section>{{end}}
        {{if .Held}}<section><h3>Held</h3><ul>{{range .Held}}<li>{{.}}</li>{{end}}</ul></section>{{end}}
        {{if .Findings}}<section class="gapped"><h3>Gapped</h3><ul>
          {{range .Findings}}
          {{if .NowPasses}}<li class="now-passes"><s>{{.Narrative}}</s> — now passes: promote to a hard assertion</li>
          {{else}}<li>{{.Narrative}} <span class="obs">— observed: {{.Detail}}</span></li>{{end}}
          {{end}}
        </ul></section>{{end}}
      </div>
      {{end}}
      <div class="steps-head">
        <span class="count">{{len .Steps}} step{{plural (len .Steps)}}</span>
      </div>
      <div class="steps"><table>
        <tr><th>section</th><th>check</th><th>verdict</th><th></th><th>detail</th></tr>
        {{range .Steps}}
        <tr>
          <td class="sec">{{.Section}}</td>
          <td>{{.Label}}</td>
          <td class="c-{{.Verdict}}">{{.Verdict}}</td>
          <td class="dur">{{.Duration}}</td>
          <td class="mono">{{.Detail}}</td>
        </tr>
        {{end}}
      </table></div>
    </div>
  </details>
{{end}}
</main>
<footer>
  generated by <a href="{{.GithubURL}}">shinari</a> ·
  <a href="{{.DocsURL}}">docs</a> ·
  <a href="{{.GithubURL}}">github</a>
</footer>
<script>
const SHINARI_MD = {{.MarkdownJS}};
function shinariCopy(ev, btn) {
  ev.preventDefault(); ev.stopPropagation(); // don't toggle the scenario
  const text = SHINARI_MD[+btn.dataset.i] || "";
  const done = () => {
    const prev = btn.textContent;
    btn.textContent = "copied"; btn.classList.add("ok");
    setTimeout(() => { btn.textContent = prev; btn.classList.remove("ok"); }, 1200);
  };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(() => fallback(text, done));
  } else {
    fallback(text, done);
  }
}
function fallback(text, done) {
  const ta = document.createElement("textarea");
  ta.value = text; ta.style.position = "fixed"; ta.style.opacity = "0";
  document.body.appendChild(ta); ta.select();
  try { document.execCommand("copy"); done(); } catch (e) {}
  document.body.removeChild(ta);
}
</script>
</body>
</html>
`))

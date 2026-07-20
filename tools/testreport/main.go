// Command testreport turns `go test -json` output (read from stdin) into a
// single self-contained HTML dashboard grouped by package, with pass/fail
// badges, durations, and the captured output of failing tests.
//
// Usage:
//
//	go test -json ./... | go run ./tools/testreport -o test-report.html
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"
	"time"
)

type event struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

type testResult struct {
	Name    string
	Status  string // pass | fail | skip
	Elapsed float64
	Output  string
}

type pkgResult struct {
	Name    string
	Status  string
	Elapsed float64
	Tests   []*testResult
	index   map[string]*testResult
}

type reportData struct {
	Title            string
	Generated        string
	Total            int
	Passed           int
	Failed           int
	Skipped          int
	DurationSeconds  float64
	PassRate         int
	Packages         []*pkgResult
	OverallStatusCSS string
	OverallStatus    string
}

func main() {
	out := flag.String("o", "test-report.html", "HTML output file")
	title := flag.String("t", "Test Report", "report title")
	flag.Parse()

	pkgs := map[string]*pkgResult{}
	getPkg := func(name string) *pkgResult {
		p, ok := pkgs[name]
		if !ok {
			p = &pkgResult{Name: name, Status: "pass", index: map[string]*testResult{}}
			pkgs[name] = p
		}
		return p
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.Package == "" {
			continue
		}
		p := getPkg(e.Package)

		if e.Test == "" {
			switch e.Action {
			case "pass", "fail", "skip":
				p.Status = e.Action
				p.Elapsed = e.Elapsed
			}
			continue
		}

		tr, ok := p.index[e.Test]
		if !ok {
			tr = &testResult{Name: e.Test}
			p.index[e.Test] = tr
			p.Tests = append(p.Tests, tr)
		}
		switch e.Action {
		case "output":
			tr.Output += e.Output
		case "pass", "fail", "skip":
			tr.Status = e.Action
			tr.Elapsed = e.Elapsed
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read input:", err)
		os.Exit(1)
	}

	data := buildReport(*title, pkgs)
	if err := render(*out, data); err != nil {
		fmt.Fprintln(os.Stderr, "render:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s  (%d tests: %d passed, %d failed, %d skipped)\n",
		*out, data.Total, data.Passed, data.Failed, data.Skipped)
	if data.Failed > 0 {
		os.Exit(1)
	}
}

func buildReport(title string, pkgs map[string]*pkgResult) reportData {
	data := reportData{Title: title, Generated: time.Now().Format("2006-01-02 15:04:05")}
	for _, p := range pkgs {
		// Keep only packages that actually ran tests.
		if len(p.Tests) == 0 {
			continue
		}
		sort.Slice(p.Tests, func(i, j int) bool { return p.Tests[i].Name < p.Tests[j].Name })
		for _, tr := range p.Tests {
			data.Total++
			data.DurationSeconds += tr.Elapsed
			switch tr.Status {
			case "pass":
				data.Passed++
			case "fail":
				data.Failed++
			case "skip":
				data.Skipped++
			}
		}
		data.Packages = append(data.Packages, p)
	}
	sort.Slice(data.Packages, func(i, j int) bool { return data.Packages[i].Name < data.Packages[j].Name })
	if data.Total > 0 {
		data.PassRate = data.Passed * 100 / data.Total
	}
	if data.Failed == 0 {
		data.OverallStatus = "PASSING"
		data.OverallStatusCSS = "ok"
	} else {
		data.OverallStatus = "FAILING"
		data.OverallStatusCSS = "fail"
	}
	return data
}

func render(path string, data reportData) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return reportTmpl.Execute(f, data)
}

var reportTmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"ms": func(sec float64) string { return fmt.Sprintf("%.0f ms", sec*1000) },
	"short": func(pkg string) string {
		if i := strings.LastIndex(pkg, "/"); i >= 0 {
			return pkg[i+1:]
		}
		return pkg
	},
}).Parse(reportHTML))

const reportHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
  :root { --ok:#1a7f37; --fail:#cf222e; --skip:#9a6700; --bg:#f6f8fa; --card:#fff; --border:#d0d7de; --text:#1f2328; --muted:#656d76; }
  * { box-sizing: border-box; }
  body { margin:0; font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif; background:var(--bg); color:var(--text); }
  .wrap { max-width:1000px; margin:0 auto; padding:32px 20px 64px; }
  h1 { font-size:22px; margin:0 0 4px; }
  .sub { color:var(--muted); font-size:13px; margin-bottom:24px; }
  .cards { display:grid; grid-template-columns:repeat(5,1fr); gap:12px; margin-bottom:24px; }
  .card { background:var(--card); border:1px solid var(--border); border-radius:10px; padding:14px 16px; }
  .card .n { font-size:24px; font-weight:700; }
  .card .l { font-size:12px; color:var(--muted); text-transform:uppercase; letter-spacing:.04em; }
  .pill { display:inline-block; padding:3px 10px; border-radius:999px; font-size:12px; font-weight:700; color:#fff; }
  .pill.ok { background:var(--ok); } .pill.fail { background:var(--fail); }
  .bar { height:8px; border-radius:999px; background:#eaeef2; overflow:hidden; margin:6px 0 24px; }
  .bar > i { display:block; height:100%; background:var(--ok); }
  details.pkg { background:var(--card); border:1px solid var(--border); border-radius:10px; margin-bottom:12px; overflow:hidden; }
  details.pkg > summary { cursor:pointer; padding:12px 16px; display:flex; align-items:center; gap:10px; list-style:none; }
  details.pkg > summary::-webkit-details-marker { display:none; }
  .pkgname { font-weight:600; }
  .pkgpath { color:var(--muted); font-size:12px; }
  .spacer { flex:1; }
  .dot { width:10px; height:10px; border-radius:50%; display:inline-block; }
  .dot.pass { background:var(--ok);} .dot.fail{ background:var(--fail);} .dot.skip{ background:var(--skip);}
  table { width:100%; border-collapse:collapse; }
  td { padding:8px 16px; border-top:1px solid var(--border); font-size:14px; }
  td.status { width:70px; }
  td.dur { width:100px; text-align:right; color:var(--muted); font-variant-numeric:tabular-nums; }
  .tag { font-size:11px; font-weight:700; padding:2px 8px; border-radius:6px; }
  .tag.pass { color:var(--ok); background:#dafbe1; } .tag.fail { color:var(--fail); background:#ffebe9; } .tag.skip{ color:var(--skip); background:#fff8c5; }
  .tname { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:13px; }
  pre.out { margin:0 16px 12px; padding:12px; background:#0d1117; color:#e6edf3; border-radius:8px; overflow:auto; font-size:12px; }
</style>
</head>
<body>
<div class="wrap">
  <h1>{{.Title}} &nbsp; <span class="pill {{.OverallStatusCSS}}">{{.OverallStatus}}</span></h1>
  <div class="sub">Generated {{.Generated}}</div>

  <div class="cards">
    <div class="card"><div class="n">{{.Total}}</div><div class="l">Tests</div></div>
    <div class="card"><div class="n" style="color:var(--ok)">{{.Passed}}</div><div class="l">Passed</div></div>
    <div class="card"><div class="n" style="color:var(--fail)">{{.Failed}}</div><div class="l">Failed</div></div>
    <div class="card"><div class="n" style="color:var(--skip)">{{.Skipped}}</div><div class="l">Skipped</div></div>
    <div class="card"><div class="n">{{ms .DurationSeconds}}</div><div class="l">Duration</div></div>
  </div>
  <div class="bar"><i style="width:{{.PassRate}}%"></i></div>

  {{range .Packages}}
  <details class="pkg" {{if ne .Status "pass"}}open{{end}}>
    <summary>
      <span class="dot {{.Status}}"></span>
      <span class="pkgname">{{short .Name}}</span>
      <span class="pkgpath">{{.Name}}</span>
      <span class="spacer"></span>
      <span class="tag {{.Status}}">{{.Status}}</span>
      <span class="dur" style="min-width:80px;text-align:right">{{ms .Elapsed}}</span>
    </summary>
    <table>
      {{range .Tests}}
      <tr>
        <td class="status"><span class="tag {{.Status}}">{{.Status}}</span></td>
        <td class="tname">{{.Name}}</td>
        <td class="dur">{{ms .Elapsed}}</td>
      </tr>
      {{if eq .Status "fail"}}<tr><td colspan="3"><pre class="out">{{.Output}}</pre></td></tr>{{end}}
      {{end}}
    </table>
  </details>
  {{end}}
</div>
</body>
</html>
`

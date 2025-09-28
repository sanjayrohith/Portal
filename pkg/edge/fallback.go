package edge

import (
	"fmt"
	"html/template"
	"net/http"
)

// FallbackTemplateData contains parameters for rendering the 404 page.
type FallbackTemplateData struct {
	Subdomain string
	Domain    string
	ErrorMsg  string
}

const fallbackHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Tunnel Not Found &mdash; Portal</title>
  <style>
    :root {
      --bg: #0d1117;
      --card-bg: #161b22;
      --border: #30363d;
      --text: #c9d1d9;
      --heading: #f0f6fc;
      --primary: #58a6ff;
      --badge-bg: rgba(248, 81, 73, 0.15);
      --badge-text: #f85149;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background-color: var(--bg);
      color: var(--text);
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      padding: 1.5rem;
    }
    .card {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 12px;
      max-width: 540px;
      width: 100%;
      padding: 2.5rem 2rem;
      box-shadow: 0 16px 32px rgba(0, 0, 0, 0.35);
      text-align: center;
    }
    .badge {
      display: inline-block;
      padding: 0.35rem 0.75rem;
      font-size: 0.85rem;
      font-weight: 600;
      border-radius: 20px;
      background-color: var(--badge-bg);
      color: var(--badge-text);
      margin-bottom: 1.25rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }
    h1 {
      color: var(--heading);
      font-size: 1.75rem;
      font-weight: 700;
      margin-bottom: 0.75rem;
    }
    p {
      font-size: 1rem;
      line-height: 1.6;
      margin-bottom: 1.5rem;
    }
    .subdomain-box {
      background: #090d13;
      border: 1px dashed var(--border);
      padding: 0.75rem 1rem;
      border-radius: 6px;
      font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
      color: var(--primary);
      font-size: 0.95rem;
      margin-bottom: 1.75rem;
      word-break: break-all;
    }
    .footer {
      font-size: 0.85rem;
      color: #8b949e;
      border-top: 1px solid var(--border);
      padding-top: 1.25rem;
    }
    .footer a {
      color: var(--primary);
      text-decoration: none;
    }
    .footer a:hover {
      text-decoration: underline;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="badge">404 Tunnel Inactive</div>
    <h1>Tunnel Not Online</h1>
    <p>The tunnel you are trying to access is not currently connected to this Portal edge.</p>
    {{if .Subdomain}}
    <div class="subdomain-box">{{.Subdomain}}{{if .Domain}}.{{.Domain}}{{end}}</div>
    {{end}}
    <p style="font-size: 0.9rem; color: #8b949e;">
      To expose a service under this subdomain, launch the Portal client with your authorized token:
    </p>
    <div class="subdomain-box" style="text-align: left; font-size: 0.85rem;">
      $ portal http 3000 {{if .Subdomain}}--subdomain {{.Subdomain}}{{end}}
    </div>
    <div class="footer">
      Powered by <strong>Portal</strong> &bull; Secure Localhost Tunneling
    </div>
  </div>
</body>
</html>`

var fallbackTmpl = template.Must(template.New("fallback").Parse(fallbackHTML))

// RenderNotFound writes a branded 404 response page to the ResponseWriter.
func RenderNotFound(w http.ResponseWriter, subdomain, domain, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNotFound)

	data := FallbackTemplateData{
		Subdomain: subdomain,
		Domain:    domain,
		ErrorMsg:  errorMsg,
	}

	if err := fallbackTmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("404 Tunnel Not Found: %s", subdomain), http.StatusNotFound)
	}
}

package loadtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type grafanaDashboard struct {
	Title    string `json:"title"`
	UID      string `json:"uid"`
	Editable bool   `json:"editable"`
	Refresh  string `json:"refresh"`
	Panels   []grafanaPanel
	Time     struct {
		From string `json:"from"`
	} `json:"time"`
	Annotations struct {
		List []struct {
			Name       string `json:"name"`
			Expression string `json:"expr"`
			TextFormat string `json:"textFormat"`
		} `json:"list"`
	} `json:"annotations"`
	Templating struct {
		List []struct {
			Name string `json:"name"`
		} `json:"list"`
	} `json:"templating"`
}

type grafanaPanel struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Datasource struct {
		UID string `json:"uid"`
	} `json:"datasource"`
	Targets []struct {
		Expression string `json:"expr"`
	} `json:"targets"`
	Options json.RawMessage `json:"options"`
}

func TestGrafanaDashboardIsProvisionable(t *testing.T) {
	root := repositoryRoot(t)
	dashboardPath := filepath.Join(root, "loadtest", "grafana", "dashboards", "goodqueue-loadtest.json")
	contents, err := os.ReadFile(dashboardPath) //nolint:gosec // Repository-relative test fixture path.
	if err != nil {
		t.Fatalf("read Grafana dashboard: %v", err)
	}

	var dashboard grafanaDashboard
	if err := json.Unmarshal(contents, &dashboard); err != nil {
		t.Fatalf("parse Grafana dashboard: %v", err)
	}
	if strings.Contains(string(contents), `"mode": "fixed-color"`) {
		t.Fatal(`Grafana 12 color mode must be "fixed", not legacy "fixed-color"`)
	}
	if dashboard.UID != "goodqueue-loadtest" || dashboard.Title == "" {
		t.Fatalf("unexpected dashboard identity: uid=%q title=%q", dashboard.UID, dashboard.Title)
	}
	if dashboard.Editable {
		t.Fatal("provisioned dashboard must be changed through Git, not Grafana UI")
	}
	if dashboard.Refresh != "5s" || dashboard.Time.From != "now-15m" {
		t.Fatalf("dashboard refresh/range=%q/%q, want 5s/now-15m", dashboard.Refresh, dashboard.Time.From)
	}
	if len(dashboard.Panels) < 10 {
		t.Fatalf("dashboard has %d panels, want at least 10", len(dashboard.Panels))
	}

	panelIDs := make(map[int]struct{}, len(dashboard.Panels))
	canvasButtons := 0
	for _, panel := range dashboard.Panels {
		if panel.ID <= 0 || panel.Title == "" {
			t.Fatalf("panel must have stable ID and title: %+v", panel)
		}
		if _, exists := panelIDs[panel.ID]; exists {
			t.Fatalf("duplicate panel ID %d", panel.ID)
		}
		panelIDs[panel.ID] = struct{}{}
		if panel.Datasource.UID != "prometheus" {
			t.Fatalf("panel %q uses datasource %q", panel.Title, panel.Datasource.UID)
		}
		if panel.Type != "canvas" && len(panel.Targets) == 0 {
			t.Fatalf("panel %q has no Prometheus targets", panel.Title)
		}
		if panel.Type == "canvas" {
			canvasButtons += validateCanvasActions(t, panel.Options)
		}
		for _, target := range panel.Targets {
			if strings.TrimSpace(target.Expression) == "" {
				t.Fatalf("panel %q has an empty PromQL expression", panel.Title)
			}
		}
	}
	if canvasButtons != 3 {
		t.Fatalf("dashboard Canvas buttons=%d, want exactly 3", canvasButtons)
	}
	annotationFound := false
	for _, annotation := range dashboard.Annotations.List {
		if annotation.Name == "Load-test lifecycle" && strings.Contains(annotation.Expression, "goodqueue_loadtest_events_total") && annotation.TextFormat == "{{event}}" {
			annotationFound = true
		}
	}
	if !annotationFound {
		t.Fatal("load-test lifecycle annotation is missing")
	}

	variables := make(map[string]struct{}, len(dashboard.Templating.List))
	for _, variable := range dashboard.Templating.List {
		variables[variable.Name] = struct{}{}
	}
	for _, required := range []string{"runId", "scenario", "keepData"} {
		if _, exists := variables[required]; !exists {
			t.Fatalf("dashboard variable %q is missing", required)
		}
	}
	text := string(contents)
	for _, required := range []string{
		`"title": "GoodQueue Load Testing"`,
		`"endpoint": "__LOADTEST_RUNNER_PUBLIC_URL__/api/v1/loadtests/runs"`,
		`\"profile\":\"smoke\"`, `\"profile\":\"medium\"`, `\"profile\":\"main\"`,
		`\"keepData\":$keepData`, `label_values(goodqueue_loadtest_current_run_info, run_id)`,
		`goodqueue_loadtest_info`, `goodqueue_http_requests_total`,
		`max(goodqueue_loadtest_last_verifier_success) or vector(0)`,
		`goodqueue_queue_waiting_capacity`, `goodqueue_loadtest_last_verifier_violations`,
		`changes(goodqueue_loadtest_events_total[10s])`,
		`last_over_time(k6_checkout_expired_outcomes_total`, `label_replace(goodqueue_loadtest_current_run_info`, `or vector(0)), 1)`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("dashboard is missing %q", required)
		}
	}
	if strings.Contains(text, "LOADTEST_RUNNER_API_KEY") || strings.Contains(text, "X-Loadtest-Api-Key") {
		t.Fatal("dashboard must not contain the runner API key")
	}
}

func TestGrafanaHistoryDashboardIsProvisionable(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "loadtest", "grafana", "dashboards", "goodqueue-loadtest-history.json")) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	var dashboard grafanaDashboard
	if err := json.Unmarshal(contents, &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.UID != "goodqueue-loadtest-history" || dashboard.Editable || len(dashboard.Panels) < 4 {
		t.Fatalf("invalid history dashboard: uid=%q editable=%t panels=%d", dashboard.UID, dashboard.Editable, len(dashboard.Panels))
	}
	text := string(contents)
	if !strings.Contains(text, `label_values(k6_http_reqs_total{keep_data=\"true\"}, testid)`) || strings.Contains(text, "loadtests/runs") {
		t.Fatal("history dashboard must select stored testids and must not start runs")
	}
}

func validateCanvasActions(t *testing.T, options json.RawMessage) int {
	t.Helper()
	var canvas struct {
		InlineEditing bool `json:"inlineEditing"`
		Root          struct {
			Elements []struct {
				Type   string `json:"type"`
				Config struct {
					API struct {
						Endpoint    string `json:"endpoint"`
						Method      string `json:"method"`
						ContentType string `json:"contentType"`
						Data        string `json:"data"`
					} `json:"api"`
				} `json:"config"`
			} `json:"elements"`
		} `json:"root"`
	}
	if err := json.Unmarshal(options, &canvas); err != nil {
		t.Fatalf("parse Canvas options: %v", err)
	}
	if canvas.InlineEditing {
		t.Fatal("Canvas inline editing must be disabled")
	}
	buttons := 0
	for _, element := range canvas.Root.Elements {
		if element.Type != "button" {
			continue
		}
		buttons++
		api := element.Config.API
		if api.Endpoint != "__LOADTEST_RUNNER_PUBLIC_URL__/api/v1/loadtests/runs" || api.Method != "POST" || api.ContentType != "application/json" {
			t.Fatalf("invalid Canvas API action: %+v", api)
		}
		if strings.Contains(api.Data, `"runId"`) || !strings.Contains(api.Data, `"scenario":"$scenario"`) || !strings.Contains(api.Data, `"keepData":$keepData`) {
			t.Fatalf("Canvas action does not use dashboard variables: %s", api.Data)
		}
	}
	return buttons
}

func TestGrafanaProvisioningUsesInternalPrometheusAndDisablesAnonymousAccess(t *testing.T) {
	root := repositoryRoot(t)
	datasource := mustReadTextFile(t, filepath.Join(
		root, "loadtest", "grafana", "provisioning", "datasources", "prometheus.yaml",
	))
	if !strings.Contains(datasource, "uid: prometheus") ||
		!strings.Contains(datasource, "url: http://prometheus:9090") ||
		!strings.Contains(datasource, "editable: false") {
		t.Fatalf("Prometheus datasource is not reproducibly provisioned:\n%s", datasource)
	}

	compose := mustReadTextFile(t, filepath.Join(root, "loadtest", "compose.loadtest.yaml"))
	for _, required := range []string{
		"grafana/grafana:12.3.6",
		"GF_AUTH_ANONYMOUS_ENABLED: \"false\"",
		"loadtest/grafana/provisioning/datasources:/etc/grafana/provisioning/datasources:ro",
		"loadtest/grafana/provisioning/dashboards:/etc/grafana/provisioning/dashboards:ro",
		"loadtest/grafana/dashboards:/etc/grafana/dashboard-templates:ro",
		`profiles: ["dev-tools"]`,
		`profiles: ["cli-loadtest"]`,
		`- "8081"`,
		"GOODQUEUE_PUBLIC_URL",
		"LOADTEST_RUNNER_PUBLIC_URL",
		"goodqueue-grafana-entrypoint.sh",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("Grafana Compose setting %q is missing", required)
		}
	}
	dashboardProvisioning := mustReadTextFile(t, filepath.Join(root, "loadtest", "grafana", "provisioning", "dashboards", "goodqueue.yaml"))
	if !strings.Contains(dashboardProvisioning, "path: /var/lib/grafana/generated-dashboards") {
		t.Fatal("Grafana must provision rendered dashboards")
	}
	entrypoint := mustReadTextFile(t, filepath.Join(root, "loadtest", "grafana", "entrypoint.sh"))
	for _, required := range []string{"__LOADTEST_RUNNER_PUBLIC_URL__", "LOADTEST_RUNNER_PUBLIC_URL", "exec /run.sh"} {
		if !strings.Contains(entrypoint, required) {
			t.Fatalf("Grafana entrypoint is missing %q", required)
		}
	}
	if strings.Contains(compose, "/var/run/docker.sock") {
		t.Fatal("loadtest compose must not expose the Docker socket")
	}
	prometheus := mustReadTextFile(t, filepath.Join(root, "loadtest", "prometheus", "prometheus.yml"))
	for _, target := range []string{"backend:8080", "loadtest-runner:8081"} {
		if !strings.Contains(prometheus, target) {
			t.Errorf("Prometheus target %q is missing", target)
		}
	}
}

func mustReadTextFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path) //nolint:gosec // Callers pass repository-relative test fixture paths.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

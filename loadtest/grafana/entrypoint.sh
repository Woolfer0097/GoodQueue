#!/bin/sh
set -eu

template=/etc/grafana/dashboard-templates/goodqueue-loadtest.json
dashboard_dir=/var/lib/grafana/generated-dashboards
dashboard="$dashboard_dir/goodqueue-loadtest.json"
runner_url=${LOADTEST_RUNNER_PUBLIC_URL:?LOADTEST_RUNNER_PUBLIC_URL is required}

case "$runner_url" in
  http://*|https://*) ;;
  *)
    echo "LOADTEST_RUNNER_PUBLIC_URL must be an absolute http(s) URL" >&2
    exit 1
    ;;
esac

# The Canvas Button implementation constructs URL directly and therefore does
# not accept a relative endpoint. Dashboard definition files themselves do not
# support Grafana environment interpolation, so render this one value at start.
escaped_runner_url=$(printf '%s' "$runner_url" | sed 's/[&|\\]/\\&/g')
mkdir -p "$dashboard_dir"
sed "s|__LOADTEST_RUNNER_PUBLIC_URL__|$escaped_runner_url|g" "$template" > "$dashboard"
cp /etc/grafana/dashboard-templates/goodqueue-loadtest-history.json "$dashboard_dir/goodqueue-loadtest-history.json"

exec /run.sh

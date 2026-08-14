#!/bin/bash

trap 'exit 0' EXIT

APP_ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
DEFAULT_CONF="$APP_ROOT/.platform/nginx/conf.d/00_realip.conf"
NO_APIGW_CONF="$APP_ROOT/.platform/nginx/conf.d/realip/00_realip_no_api_gateway.conf"
LOG=/var/log/realip-select.log

# environments with no API Gateway in front
NO_APIGW_ENVS="Otel-collector-env-1"

log(){ echo "$(date -u) realip-select: $*" >> "$LOG" 2>/dev/null; }

env_name=$(/opt/elasticbeanstalk/bin/get-config container -k environment_name 2>/dev/null)

case " $NO_APIGW_ENVS " in
    *" $env_name "*)
        if cp "$NO_APIGW_CONF" "$DEFAULT_CONF" 2>>"$LOG"; then
            log "${env_name}: installed no-API-Gateway config"
        else
            log "${env_name}: ERROR copying variant, keeping default config"
        fi
        ;;
    *)
        log "${env_name:-unknown}: keeping default API Gateway config"
        ;;
esac

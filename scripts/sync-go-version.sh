#!/usr/bin/env bash

set -euo pipefail

goVersion="$(grep "^golang " .tool-versions | awk '{print $2}')"
sedExpr="s#\(FROM .*golang\):[.0-9]\+ \(AS builder\)#\1:$goVersion \2#"

case "$OSTYPE" in
linux*)
  sed -i -e "$sedExpr" Dockerfile
  ;;
darwin*)
  sed -i '' Dockerfile
  ;;
*)
  echo "Unsupported OS" >&2
  exit 1
  ;;
esac

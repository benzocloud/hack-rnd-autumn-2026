#!/bin/sh
set -eu
gh workflow run stack-deploy.yml --repo benzocloud/hack-rnd-autumn-2026 --ref main
gh workflow run landing.yml --repo benzocloud/hack-rnd-autumn-2026 --ref main

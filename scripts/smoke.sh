#!/bin/sh
set -eu

exec vfox exec nodejs@24.20.0 -- node scripts/smoke.mjs "$@"

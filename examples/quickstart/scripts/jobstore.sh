#!/bin/sh
# SPDX-FileCopyrightText: 2026 The Shinari Authors
# SPDX-License-Identifier: Apache-2.0
# A toy job store: state is files under $JOBSTORE_DIR. Deliberately has one
# resilience gap: recovery re-runs the whole job, duplicating work.
set -eu
DIR=${JOBSTORE_DIR:-.jobstore}
cmd=$1
shift
case "$cmd" in
  reset)
    rm -rf "$DIR" && mkdir -p "$DIR"
    ;;
  healthy)
    test -d "$DIR" && echo ok
    ;;
  submit)
    job=$1
    echo RUNNING > "$DIR/$job.state"
    echo run >> "$DIR/$job.runs"
    ;;
  complete)
    job=$1
    echo SUCCESS > "$DIR/$job.state"
    ;;
  crash)
    # The worker dies mid-task: the job is left RUNNING, orphaned.
    job=$1
    echo RUNNING > "$DIR/$job.state"
    ;;
  recover)
    # A peer picks the orphan up — and re-runs it from scratch (the gap).
    job=$1
    echo run >> "$DIR/$job.runs"
    echo SUCCESS > "$DIR/$job.state"
    ;;
  status)
    job=$1
    cat "$DIR/$job.state"
    ;;
  runs)
    job=$1
    wc -l < "$DIR/$job.runs" | tr -d ' '
    ;;
  *)
    echo "unknown command: $cmd" >&2
    exit 2
    ;;
esac

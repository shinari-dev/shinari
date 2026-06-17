// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/shinari-dev/shinari/core/model"
)

// runRepeat executes a `repeat` step: it runs the `do:` sequence `times`
// iterations in order. Each iteration runs the body via runSection, so inner
// steps flow back through runStep individually (dry-run skip, verdict split,
// fault tracking, findings, and events all apply per step). repeat is
// count-based only, never duration, so it never reintroduces wall-clock timing.
func (r *runner) runRepeat(ctx context.Context, section, phase string, st *model.Step, finish func(CheckVerdict, string) StepResult) StepResult {
	var w struct {
		Times      int          `yaml:"times"`
		StopOnFail *bool        `yaml:"stopOnFail"`
		Do         []model.Step `yaml:"do"`
	}
	if err := st.With.Decode(&w); err != nil {
		return finish(CheckFail, fmt.Sprintf("repeat: bad with: %v", err))
	}
	if w.Times < 1 {
		return finish(CheckFail, "repeat: times must be >= 1")
	}
	if len(w.Do) == 0 {
		return finish(CheckFail, "repeat: do must be a non-empty list")
	}
	for i := range w.Do {
		w.Do[i].File = st.File // inherit file for error messages
	}
	stopOnFail := true
	if w.StopOnFail != nil {
		stopOnFail = *w.StopOnFail
	}
	times := w.Times
	if r.opts.DryRun {
		times = 1 // one pass previews the body; N skip-passes only add journal noise
	}

	var failures []string
	for i := 0; i < times; i++ {
		before := snapshotBg(r.bg)
		msg, ok := r.runSection(ctx, section, phase, w.Do, stopOnFail)
		if !ok {
			failures = append(failures, msg)
			r.cancelBackgroundsSince(before) // a generator must not leak past an aborted iteration
			if stopOnFail {
				break
			}
		}
	}
	if len(failures) > 0 {
		return finish(CheckFail, strings.Join(failures, "; "))
	}
	return finish(CheckPass, "")
}

// snapshotBg records the background-task names live before an iteration.
func snapshotBg(bg map[string]*bgHandle) map[string]bool {
	s := make(map[string]bool, len(bg))
	for name := range bg {
		s[name] = true
	}
	return s
}

// cancelBackgroundsSince cancels and removes any background started during an
// aborted iteration (not present in the pre-iteration snapshot), so a generator
// cannot leak into the recovery, verify, or teardown phases.
func (r *runner) cancelBackgroundsSince(before map[string]bool) {
	for name, h := range r.bg {
		if before[name] {
			continue
		}
		h.cancel()
		<-h.done
		delete(r.bg, name)
	}
}

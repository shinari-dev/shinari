// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package engine is the execution pipeline: it runs scenarios against a
// registry and emits data — a typed event stream plus a structured Result.
// It never prints, never reads argv, never owns an exit code.
package engine

import "time"

type EventType string

const (
	EvScenarioStarted  EventType = "scenario.started"
	EvPhaseStarted     EventType = "phase.started"
	EvStepStarted      EventType = "step.started"
	EvStepPassed       EventType = "step.passed"
	EvStepFailed       EventType = "step.failed"
	EvStepSkipped      EventType = "step.skipped"
	EvFaultInjected    EventType = "fault.injected"
	EvGateObserved     EventType = "gate.observed"
	EvFindingRecorded  EventType = "finding.recorded"
	EvScenarioFinished EventType = "scenario.finished"
)

// stepEventType maps a check verdict to the step event it emits — a
// FINDING is reported on the wire as a failure (renderers tell it apart by
// the verdict in the payload).
var stepEventType = map[CheckVerdict]EventType{
	CheckPass:    EvStepPassed,
	CheckFail:    EvStepFailed,
	CheckSkip:    EvStepSkipped,
	CheckFinding: EvStepFailed,
}

// Event is one entry of the append-only, ordered stream. Result is its
// deterministic reduction.
type Event struct {
	Type     EventType      `json:"type"`
	Time     time.Time      `json:"time"`
	Scenario string         `json:"scenario,omitempty"`
	Section  string         `json:"section,omitempty"`
	Phase    string         `json:"phase,omitempty"`
	Step     string         `json:"step,omitempty"`
	Verb     string         `json:"verb,omitempty"`
	Payload  map[string]any `json:"payload,omitempty"`
}

// Emitter receives events live during a run.
type Emitter interface {
	Emit(Event)
}

// EmitterFunc adapts a function to Emitter.
type EmitterFunc func(Event)

func (f EmitterFunc) Emit(e Event) { f(e) }

// Recorder is an Emitter that keeps the stream for replay/reduction.
type Recorder struct {
	Events []Event
}

func (r *Recorder) Emit(e Event) { r.Events = append(r.Events, e) }

// Multi fans an event out to several emitters in order.
func Multi(emitters ...Emitter) Emitter {
	return EmitterFunc(func(e Event) {
		for _, em := range emitters {
			if em != nil {
				em.Emit(e)
			}
		}
	})
}

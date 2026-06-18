package testutil

import (
	"context"
	"strings"
	"sync"

	vyosapply "github.com/Telecominfraproject/olg-renderer-vyos/apply"
)

// FakeApplyBackend is a controllable VyOS apply backend test double.
type FakeApplyBackend struct {
	Plan      vyosapply.Plan
	UsePlan   bool
	Result    vyosapply.Result
	UseResult bool

	PrepareErr error
	ApplyErr   error

	ValidatePrepare func(vyosapply.Input) error
	ValidateApply   func(vyosapply.Input) error

	MutateOnPrepare    bool
	MutatePrepareInput func(vyosapply.Input) vyosapply.Input

	Events *EventRecorder

	mu            sync.Mutex
	prepareCalls  int
	applyCalls    int
	prepareInputs []vyosapply.Input
	applyInputs   []vyosapply.Input
}

func (f *FakeApplyBackend) Prepare(ctx context.Context, input vyosapply.Input) (vyosapply.Plan, error) {
	if f.Events != nil {
		f.Events.Record("prepare")
	}

	recorded := input
	f.mu.Lock()
	if f.MutateOnPrepare {
		mutate := f.MutatePrepareInput
		if mutate == nil {
			mutate = func(in vyosapply.Input) vyosapply.Input {
				in.DesiredCommands = strings.TrimRight(in.DesiredCommands, "\n") + "\n# mutated by FakeApplyBackend Prepare\n"
				return in
			}
		}
		recorded = mutate(recorded)
	}
	f.prepareCalls++
	f.prepareInputs = append(f.prepareInputs, recorded)
	validate := f.ValidatePrepare
	err := f.PrepareErr
	plan := f.Plan
	usePlan := f.UsePlan
	f.mu.Unlock()

	if validate != nil {
		if err := validate(input); err != nil {
			return vyosapply.Plan{}, err
		}
	}
	if err != nil {
		return vyosapply.Plan{}, err
	}
	if usePlan {
		return cloneApplyPlan(plan), nil
	}
	return vyosapply.Plan{
		Target:     input.Target,
		ConfigUUID: input.ConfigUUID,
		Commit:     true,
	}, nil
}

func (f *FakeApplyBackend) Apply(ctx context.Context, input vyosapply.Input) (vyosapply.Result, error) {
	if f.Events != nil {
		f.Events.Record("apply")
	}

	f.mu.Lock()
	f.applyCalls++
	f.applyInputs = append(f.applyInputs, input)
	validate := f.ValidateApply
	err := f.ApplyErr
	result := f.Result
	useResult := f.UseResult
	f.mu.Unlock()

	if validate != nil {
		if err := validate(input); err != nil {
			return vyosapply.Result{}, err
		}
	}
	if err != nil {
		return vyosapply.Result{}, err
	}
	if useResult {
		return cloneApplyResult(result), nil
	}
	return vyosapply.Result{
		Target:     input.Target,
		ConfigUUID: input.ConfigUUID,
		Applied:    true,
	}, nil
}

func (f *FakeApplyBackend) PrepareCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prepareCalls
}

func (f *FakeApplyBackend) ApplyCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.applyCalls
}

func (f *FakeApplyBackend) PrepareInputs() []vyosapply.Input {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]vyosapply.Input(nil), f.prepareInputs...)
}

func (f *FakeApplyBackend) ApplyInputs() []vyosapply.Input {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]vyosapply.Input(nil), f.applyInputs...)
}

func (f *FakeApplyBackend) LastPrepareInput() (vyosapply.Input, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.prepareInputs) == 0 {
		return vyosapply.Input{}, false
	}
	return f.prepareInputs[len(f.prepareInputs)-1], true
}

func (f *FakeApplyBackend) LastApplyInput() (vyosapply.Input, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.applyInputs) == 0 {
		return vyosapply.Input{}, false
	}
	return f.applyInputs[len(f.applyInputs)-1], true
}

func (f *FakeApplyBackend) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.prepareCalls = 0
	f.applyCalls = 0
	f.prepareInputs = nil
	f.applyInputs = nil
}

func cloneApplyPlan(in vyosapply.Plan) vyosapply.Plan {
	in.DeleteCommands = append([]string(nil), in.DeleteCommands...)
	in.SetCommands = append([]string(nil), in.SetCommands...)
	return in
}

func cloneApplyResult(in vyosapply.Result) vyosapply.Result {
	in.DeleteCommands = append([]string(nil), in.DeleteCommands...)
	in.SetCommands = append([]string(nil), in.SetCommands...)
	return in
}

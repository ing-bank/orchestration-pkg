package task

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Struct definition required to satisfy the Runnable interface in combination
// with the Run function below.
type Task struct {
	sleep int32
}

// Example work function. Even sleep numbers will fail immediately.
// Other numbers will be a millisecond sleep. Satisfies Runnable
// interface.
func (task Task) Run(_ context.Context) error {
	if task.sleep%2 == 0 {
		return errors.New("failed")
	}
	if task.sleep%29 == 0 {
		panic("failed")
	}
	time.Sleep(time.Duration(task.sleep) * time.Millisecond)
	return nil // Success
}

// Struct definition required to satisfy the Runnable interface in combination
// with the Run function below.
type InstantTask struct{}

// A task with minimal overhead
func (task InstantTask) Run(_ context.Context) error {
	return nil
}

func TestRun(t *testing.T) {
	tasks := []Runnable{
		&Task{33},   // Will succeed
		&Task{20},   // Will fail, because even number check in test Task
		&Task{1337}, // Will timeout (see 100ms limit Context below)
		&Task{29},    // Will panic, because 29 number check in test Task
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	errs := Run(tasks, ctx) // <- Function to be tested

	// Assertions
	if elapsed := time.Now().Sub(start); elapsed > (200*time.Millisecond) || elapsed < (90*time.Millisecond) {
		t.Errorf("Expected to finish in approximately 100ms, but it took %v\n", elapsed)
	}
	if len(errs) != len(tasks) {
		t.Errorf("Expected result for every task, got %d expected %d\n", len(errs), len(tasks))
	}
	if errs[0] != nil {
		t.Errorf("Expected task 0 to succeed, but got \"%v\"\n", errs[0])
	}
	if errs[1] == nil || errs[1].Error() != "failed" {
		t.Errorf("Expected task 1 error to be \"failed\", but got \"%v\"\n", errs[1])
	}
	if errs[2] == nil || errs[2].Error() != "timeout" {
		t.Errorf("Expected task 2 error to be \"timeout\", but got \"%v\"\n", errs[2])
	}
	if errs[3] == nil || errs[3].Error() != "internal server error" {
		t.Errorf("Expected task 3 error to be \"internal server error\", but got \"%v\"\n", errs[3])
	}
}

func TestRunWithInstantTasks(t *testing.T) {
	// We create a large amount of "instant tasks", which means tasks will be finished while others haven't
	// even started yet. The Run function should have no problem with this.
	var instantTasks []Runnable
	for i := 0; i < 9999; i++ {
		instantTasks = append(instantTasks, &InstantTask{})
	}
	if AnyError(Run(instantTasks, context.TODO())) {
		t.Errorf("No task can throw an error")
	}
}

func TestAnyError(t *testing.T) {
	errs := []error{
		nil, errors.New("one"), nil,
	}
	if !AnyError(errs) {
		t.Errorf("Should have detected an error\n")
	}
	if AnyError([]error{}) {
		t.Errorf("Empty error list should have no errors\n")
	}
}

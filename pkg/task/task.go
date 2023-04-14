// Package task contains functions that strive to make it easier to schedule goroutines
// and gather the (error) results. The main focus is that regardless of whether a goroutine
// finishes in time, or successfully, there is a traceable result. That means it is
// easy to track which goroutines finished, and also those that did not (successfully).
//
// The core of this package is a Runnable. The error status of a Runnable implementation
// is set automatically by the return value, and they are aggregated by the Run function.
// A Runnable implementation only takes a context as an argument, so any other parameters
// should be kept inside your struct.
//
// Take the following example:
//
//  type Task struct { ... } // Your concurrent task
//  func (task Task) Run(ctx context.Context) error { ... } // Implement Runnable
//
//  func example() {
//      tasks := []Runnable{ &Task{ ... }, ...} // Tasks you wish to be executed concurrently
//      errs := Run(tasks, context.TODO()) // Run 'tasks' concurrently, context will be passed to each task
//      ... // Do something with errors, probably
//  }
//
package task

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
)

// Runnable structs can be passed to the Run function below, for context aware concurrent execution.
type Runnable interface {
	Run(context.Context) error
}

// Run runs multiple tasks until completion, using Run, or when context is done.
// Each task may return an optional error, the returned error list
// is in-order as the given task list and may contain nil values.
func Run(tasks []Runnable, ctx context.Context) []error {
	var wg sync.WaitGroup
	wg.Add(len(tasks))
	var taskErrors = make([]error, len(tasks))

	for i := 0; i < len(tasks); i++ {
		count := i // Scoping

		go func() {
			workerChan := make(chan error, 1)
			defer wg.Done()

			go func() {
				defer close(workerChan)
				defer func() {
					if err := recover(); err != nil {
						fmt.Printf("[CRITICAL] Recovering from exception in task: %v %s\n", err, string(debug.Stack()))
						workerChan <- errors.New("internal server error")
					}
				}()
				workerChan <- tasks[count].Run(ctx)
			}()

			select {
			case err := <-workerChan: // Task is finished
				taskErrors[count] = err
			case <-ctx.Done():
				taskErrors[count] = errors.New("timeout")
			}
		}()
	}

	wg.Wait()
	return taskErrors
}

// AnyError returns true when there is any non-nil value in the list of errors provided, otherwise false
func AnyError(errors []error) bool {
	for _, err := range errors {
		if err != nil {
			return true
		}
	}
	return false
}

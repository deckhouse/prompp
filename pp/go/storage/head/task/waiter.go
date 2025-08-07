package task

import "errors"

//
// Task
//

// Task the minimum required Task implementation.
type Task interface {
	Wait() error
}

//
// TaskWaiter
//

// Waiter aggregates the wait for tasks to be completed.
type Waiter[TTask Task] struct {
	tasks []TTask
}

// NewTaskWaiter init new TaskWaiter for n task.
func NewTaskWaiter[TTask Task](n int) *Waiter[TTask] {
	return &Waiter[TTask]{
		tasks: make([]TTask, 0, n),
	}
}

// Add task to waiter.
func (tw *Waiter[TTask]) Add(t TTask) {
	tw.tasks = append(tw.tasks, t)
}

// Wait for tasks to be completed.
func (tw *Waiter[TTask]) Wait() error {
	errs := make([]error, len(tw.tasks))
	for _, t := range tw.tasks {
		errs = append(errs, t.Wait())
	}

	return errors.Join(errs...)
}

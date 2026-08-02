package app

import "context"

// Start is intentionally empty until a clean-slate module registers a worker.
func (a *Application) Start(context.Context) {}

// Stop is retained as the application lifecycle boundary.
func (a *Application) Stop() {}

func (a *Application) StopContext(context.Context) error { return nil }

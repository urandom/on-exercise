package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urandom/on-exercise/internal/state"
	"github.com/urandom/on-exercise/internal/store"
)

var errInjected = errors.New("injected write failure (-fail-on)")

type failingStore struct {
	*store.JSONStore
	target state.EmployeeID
}

func (s failingStore) check(id state.EmployeeID) error {
	if id == s.target {
		return fmt.Errorf("%w: employee %q", errInjected, id)
	}

	return nil
}

func (s failingStore) CreateAccount(ctx context.Context, id state.EmployeeID) error {
	if err := s.check(id); err != nil {
		return err
	}

	return s.JSONStore.CreateAccount(ctx, id)
}

func (s failingStore) SetEnabled(ctx context.Context, id state.EmployeeID, enabled bool) error {
	if err := s.check(id); err != nil {
		return err
	}

	return s.JSONStore.SetEnabled(ctx, id, enabled)
}

func (s failingStore) AddGroup(ctx context.Context, id state.EmployeeID, group state.Group) error {
	if err := s.check(id); err != nil {
		return err
	}

	return s.JSONStore.AddGroup(ctx, id, group)
}

func (s failingStore) RemoveGroup(ctx context.Context, id state.EmployeeID, group state.Group) error {
	if err := s.check(id); err != nil {
		return err
	}

	return s.JSONStore.RemoveGroup(ctx, id, group)
}

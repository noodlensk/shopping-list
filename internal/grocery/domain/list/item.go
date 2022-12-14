package list

import (
	"context"
	"fmt"
)

type Item struct {
	Name   string
	Bought bool
}

type Repository interface {
	AddItem(ctx context.Context, name string) error
	UpdateItem(
		ctx context.Context,
		name string,
		updateFn func(ctx context.Context, i *Item) (*Item, error),
	) error
	DeleteItem(ctx context.Context, name string) error
	ListItems(ctx context.Context) (items []Item, err error)
}

type NotFoundError struct{ Item string }

func (e NotFoundError) Error() string {
	return fmt.Sprintf("item %q not found", e.Item)
}
